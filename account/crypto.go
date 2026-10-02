package account

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/x402"
)

// Each authorization belongs to one account before anything is signed. Never
// accept a customer-supplied transaction as proof that their account paid.
type cryptoPayment struct {
	Account     string                   `json:"account"`
	Created     string                   `json:"account_created"`
	Credits     int                      `json:"credits"`
	Nonce       string                   `json:"nonce"`
	Before      int64                    `json:"before"`
	Block       uint64                   `json:"block"`
	Requirement x402.PaymentRequirements `json:"requirement"`
	From        string                   `json:"from"`
	Signature   string                   `json:"signature,omitempty"`
	Transaction string                   `json:"transaction,omitempty"`
	Status      string                   `json:"status"`
}

var cryptoStore = struct {
	sync.Mutex
	payments map[string]cryptoPayment
	err      error
}{payments: map[string]cryptoPayment{}}
var cryptoRunning sync.Map
var cryptoLocks sync.Map
var cryptoWake = make(chan string, 64)

// CryptoConfigured advertises only native USDC on Base, the checkout supported
// by the browser signer and receipt verifier.
func CryptoConfigured() bool { return cryptoRequirement(100) != nil }

func cryptoRequirement(credits int) *x402.PaymentRequirements {
	r := x402.TopUpRequirement(credits)
	if r == nil || x402.NormalizeNetwork(r.Network) != "eip155:8453" || !strings.EqualFold(r.Asset, baseUSDC) || r.Extra["name"] != "USD Coin" || r.Extra["version"] != "2" || !cryptoAddress(r.PayTo) {
		return nil
	}
	r.MaxTimeoutSeconds = 600
	return r
}

// StartCryptoPayments resumes pending settlements independently of open pages.
func StartCryptoPayments() {
	cryptoStore.Lock()
	cryptoStore.err = data.LoadJSON("crypto_payments.json", &cryptoStore.payments)
	if errors.Is(cryptoStore.err, os.ErrNotExist) {
		cryptoStore.err = nil
	}
	if cryptoStore.payments == nil {
		cryptoStore.payments = map[string]cryptoPayment{}
	}
	cryptoStore.Unlock()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			cryptoStore.Lock()
			var ids []string
			if cryptoStore.err == nil {
				for id, p := range cryptoStore.payments {
					if p.Status == "pending" {
						ids = append(ids, id)
					}
				}
			}
			cryptoStore.Unlock()
			for _, id := range ids {
				settleCrypto(id)
			}
			select {
			case id := <-cryptoWake:
				settleCrypto(id)
			case <-ticker.C:
			}
		}
	}()
}

func cryptoLock(id string) func() {
	lock, _ := cryptoLocks.LoadOrStore(id, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func saveCrypto(p cryptoPayment) error {
	cryptoStore.Lock()
	defer cryptoStore.Unlock()
	old, exists := cryptoStore.payments[p.Account]
	cryptoStore.payments[p.Account] = p
	if err := data.SaveJSON("crypto_payments.json", cryptoStore.payments); err != nil {
		if exists {
			cryptoStore.payments[p.Account] = old
		} else {
			delete(cryptoStore.payments, p.Account)
		}
		return err
	}
	return nil
}

func cryptoAddress(s string) bool {
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	return strings.HasPrefix(s, "0x") && err == nil && len(b) == 20 && strings.Trim(s[2:], "0") != ""
}

func cryptoPage(r *http.Request) string {
	return `<section id="crypto" class="card section-stack"><h2>Pay with crypto</h2><p>Pay directly from your wallet with USDC on Base. 1 USDC buys 100 credits.</p><form id="crypto-checkout" class="form" method="post" action="/account/crypto" data-csrf="` + html.EscapeString(auth.CSRFToken(r)) + `"><label for="crypto-amount">Amount (USDC)</label><input id="crypto-amount" name="amount" type="number" min="1" max="500" step="1" value="10" required><p id="crypto-total">10 USDC = 1,000 credits</p><div class="form-actions"><button type="submit">Pay with crypto</button></div><p id="crypto-status" role="status"></p></form><p class="text-muted">Use a browser wallet or open this page in your wallet’s browser. Confirm the payment in your wallet; no deposit into Micro’s wallet is needed.</p><noscript>JavaScript and a compatible wallet are needed for crypto checkout.</noscript></section>`
}

// CryptoHandler creates a request, accepts its signature, or reads its status.
// Signed terms come from our durable record, never from the browser's payload.
func CryptoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json")
	sess, acc, err := auth.RequireSession(r)
	fail := func(code int, message string) {
		w.WriteHeader(code)
		app.RespondJSON(w, map[string]string{"error": message})
	}
	if err != nil || sess == nil || acc == nil || sess.Type != "account" {
		fail(401, "Sign in to pay with crypto.")
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	if r.Method == "POST" && !auth.StrictCSRF(r) {
		fail(403, "Reload the page and try again.")
		return
	}
	var in struct {
		Amount    string `json:"amount"`
		From      string `json:"from"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if r.Method == "POST" {
		if err := auth.CheckPostRate(acc.ID); err != nil {
			fail(429, err.Error())
			return
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			fail(400, "Invalid payment request.")
			return
		}
	}
	unlock := cryptoLock(acc.ID)
	defer unlock()
	cryptoStore.Lock()
	loadErr := cryptoStore.err
	p, exists := cryptoStore.payments[acc.ID]
	cryptoStore.Unlock()
	if loadErr != nil {
		fail(503, "Crypto payments are temporarily unavailable.")
		return
	}
	created := strconv.FormatInt(acc.Created.UnixNano(), 10)
	if exists && p.Created != created {
		fail(409, "A previous payment needs review. Contact support.")
		return
	}
	if r.Method == "POST" && in.Signature != "" {
		if !exists || p.Nonce != in.Nonce {
			fail(409, "This payment request has changed. Reload the page.")
			return
		}
		if p.Status == "new" {
			sig, e := hex.DecodeString(strings.TrimPrefix(in.Signature, "0x"))
			if e != nil || len(sig) != 65 || !strings.HasPrefix(in.Signature, "0x") {
				fail(400, "Invalid wallet signature.")
				return
			}
			if time.Now().Unix() >= p.Before {
				fail(409, "This request expired. Start a new payment.")
				return
			}
			p.Signature = in.Signature
			if err := x402.VerifySigned(cryptoPayload(p), &p.Requirement); err != nil {
				app.Log("account", "crypto verification failed for %s: %v", acc.ID, err)
				fail(400, "The payment could not be verified. Check your USDC balance on Base and try again.")
				return
			}
			p.Status = "pending"
			if err := saveCrypto(p); err != nil {
				fail(503, "Could not save the payment. Nothing has been submitted.")
				return
			}
		}
		if p.Status == "pending" {
			select {
			case cryptoWake <- acc.ID:
			default:
			}
		}
	} else if r.Method == "POST" {
		if !CryptoConfigured() {
			fail(503, "Crypto checkout is not available on this instance.")
			return
		}
		if exists && p.Status == "pending" {
			fail(409, "Your previous payment is still being checked. Do not pay again.")
			return
		}
		dollars, e := strconv.Atoi(in.Amount)
		if e != nil || dollars < 1 || dollars > maxTopupDollars || !cryptoAddress(in.From) {
			fail(400, "Enter 1–500 USDC and connect a valid wallet.")
			return
		}
		requirement := cryptoRequirement(dollars * 100)
		if requirement == nil {
			fail(503, "Crypto checkout is not available on this instance.")
			return
		}
		// Read the chain before authorizing a payment so recovery has a bounded range.
		block, e := cryptoBlock()
		if e != nil {
			fail(503, "Could not reach the payment network. Try again shortly.")
			return
		}
		nonce := make([]byte, 32)
		if _, e = rand.Read(nonce); e != nil {
			fail(503, "Could not create payment.")
			return
		}
		p = cryptoPayment{Account: acc.ID, Created: created, Credits: dollars * 100, Nonce: "0x" + hex.EncodeToString(nonce), Before: time.Now().Unix() + 600, Block: block, Requirement: *requirement, From: strings.ToLower(in.From), Status: "new"}
		if e = saveCrypto(p); e != nil {
			fail(503, "Could not save payment request.")
			return
		}
	}
	if p.Nonce == "" {
		app.RespondJSON(w, map[string]string{"status": "none"})
		return
	}
	app.RespondJSON(w, map[string]any{"status": p.Status, "nonce": p.Nonce, "credits": p.Credits, "transaction": p.Transaction, "typedData": cryptoTypedData(p)})
}

func cryptoAuthorization(p cryptoPayment) map[string]string {
	return map[string]string{"from": p.From, "to": p.Requirement.PayTo, "value": p.Requirement.AmountAtomic(), "validAfter": "0", "validBefore": strconv.FormatInt(p.Before, 10), "nonce": p.Nonce}
}

func cryptoTypedData(p cryptoPayment) map[string]any {
	type field struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	return map[string]any{
		"domain":      map[string]any{"name": "USD Coin", "version": "2", "chainId": 8453, "verifyingContract": p.Requirement.Asset},
		"primaryType": "TransferWithAuthorization",
		"types": map[string]any{
			"EIP712Domain":              []field{{"name", "string"}, {"version", "string"}, {"chainId", "uint256"}, {"verifyingContract", "address"}},
			"TransferWithAuthorization": []field{{"from", "address"}, {"to", "address"}, {"value", "uint256"}, {"validAfter", "uint256"}, {"validBefore", "uint256"}, {"nonce", "bytes32"}},
		}, "message": cryptoAuthorization(p),
	}
}

func settleCrypto(id string) {
	if _, running := cryptoRunning.LoadOrStore(id, true); running {
		return
	}
	defer cryptoRunning.Delete(id)
	unlock := cryptoLock(id)
	defer unlock()
	cryptoStore.Lock()
	p := cryptoStore.payments[id]
	cryptoStore.Unlock()
	if p.Status != "pending" {
		return
	}
	acc, err := auth.GetAccount(id)
	if err != nil || strconv.FormatInt(acc.Created.UnixNano(), 10) != p.Created {
		return
	}
	tx := p.Transaction
	if tx == "" {
		// Recover a broadcast whose facilitator response was lost before retrying.
		tx, err = cryptoReceipt(p)
		if errors.Is(err, errCryptoExpired) {
			p.Status = "expired"
			p.Signature = ""
			saveErr := saveCrypto(p)
			if saveErr != nil {
				app.Log("account", "crypto expiry save failed: %v", saveErr)
			}
			return
		}
		if err != nil {
			app.Log("account", "crypto receipt check failed: %v", err)
			return
		}
		if tx == "" && time.Now().Unix() < p.Before {
			res, e := x402.SettleSigned(cryptoPayload(p), &p.Requirement)
			if e != nil {
				app.Log("account", "crypto settlement pending for %s: %v", id, e)
				return
			}
			if !res.Success || !cryptoHash(res.Transaction) || x402.NormalizeNetwork(res.Network) != "eip155:8453" {
				return
			}
			tx = strings.ToLower(res.Transaction)
		}
	}
	if tx == "" {
		return
	} // Keep uncertain outcomes recoverable; never ask for another payment.
	p.Transaction = tx
	if err := saveCrypto(p); err != nil {
		return
	}
	_, err = CreditOnce(id, p.Credits, "topup_usdc", "crypto:"+p.Nonce, map[string]interface{}{"source": "usdc", "tx": tx, "network": p.Requirement.Network, "from": p.From})
	if err != nil {
		app.Log("account", "crypto credit pending for %s: %v", id, err)
		return
	}
	p.Status = "paid"
	p.Signature = ""
	if err := saveCrypto(p); err != nil {
		app.Log("account", "crypto receipt save failed: %v", err)
	}
}

func cryptoHash(s string) bool {
	b, e := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	return strings.HasPrefix(s, "0x") && e == nil && len(b) == 32
}

var errCryptoExpired = errors.New("payment authorization expired")

func cryptoPayload(p cryptoPayment) string {
	payload := map[string]any{"x402Version": 2, "accepted": p.Requirement, "payload": map[string]any{"signature": p.Signature, "authorization": cryptoAuthorization(p)}}
	if p.Requirement.Amount == "" {
		payload["x402Version"] = 1
		delete(payload, "accepted")
		payload["scheme"] = "exact"
		payload["network"] = p.Requirement.Network
	}
	raw, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(raw)
}
