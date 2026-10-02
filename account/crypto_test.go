package account

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/x402"
)

func cryptoFixture(t *testing.T) (*auth.Account, func(string, string, bool) *httptest.ResponseRecorder) {
	t.Helper()
	t.Setenv("X402_PAY_TO", "0x1111111111111111111111111111111111111111")
	t.Setenv("X402_NETWORK", "eip155:8453")
	t.Setenv("X402_VERSION", "2")
	t.Setenv("X402_ASSETS", "USDC")
	old, oldErr := cryptoStore.payments, cryptoStore.err
	cryptoStore.payments, cryptoStore.err = map[string]cryptoPayment{}, nil
	t.Cleanup(func() { cryptoStore.payments, cryptoStore.err = old, oldErr })
	acc := &auth.Account{ID: "crypto_" + t.Name(), Approved: true, Created: time.Now()}
	auth.SetAccountForTest(acc)
	sess, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		auth.Logout(sess.Token)
		auth.RemoveAccountForTest(acc.ID)
		withLedger(func(l *ledger) bool { delete(transactions, acc.ID); delete(balances, acc.ID); return true })
	})
	call := func(method, body string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://micro.example/account/crypto", strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		CryptoHandler(w, r)
		return w
	}
	return acc, call
}

func TestCryptoPaymentRecoveryAndIsolation(t *testing.T) {
	acc, call := cryptoFixture(t)
	oldHTTP, oldTransport := cryptoHTTP, http.DefaultTransport
	defer func() { cryptoHTTP = oldHTTP; http.DefaultTransport = oldTransport }()
	var broadcast atomic.Int32
	var onchain atomic.Bool
	tx := "0x" + strings.Repeat("a", 64)
	var payment cryptoPayment
	cryptoHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "eth_chainId":
			result = "0x2105"
		case "eth_blockNumber":
			result = "0x100"
		case "eth_getBlockByNumber":
			result = map[string]string{"number": "0x200", "timestamp": fmt.Sprintf("0x%x", time.Now().Unix())}
		case "eth_getLogs":
			logs := []cryptoLog{}
			if onchain.Load() {
				logs = append(logs, cryptoLog{Address: baseUSDC, Topics: []string{cryptoTopic("AuthorizationUsed(address,bytes32)"), cryptoAddressTopic(payment.From), payment.Nonce}, Transaction: tx})
			}
			result = logs
		case "eth_getTransactionReceipt":
			result = map[string]any{"status": "0x1", "transactionHash": tx, "logs": []cryptoLog{cryptoTransferFixture(payment)}}
		default:
			t.Errorf("unexpected RPC %s", req.Method)
		}
		return stripeResponse(map[string]any{"result": result}), nil
	})}
	http.DefaultTransport = stripeTransport(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Requirement struct {
				Amount string `json:"amount"`
			} `json:"paymentRequirements"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Requirement.Amount != "10000000" {
			t.Errorf("browser changed amount: %q", body.Requirement.Amount)
		}
		switch r.URL.Path {
		case "/platform/v2/x402/verify":
			return stripeResponse(map[string]any{"isValid": true}), nil
		case "/platform/v2/x402/settle":
			broadcast.Add(1)
			return stripeResponse(map[string]any{"success": true, "transaction": tx, "network": "eip155:8453"}), nil
		default:
			t.Errorf("unexpected facilitator path %s", r.URL.Path)
			return nil, fmt.Errorf("unexpected endpoint")
		}
	})
	if w := call("POST", `{"amount":"10","from":"0x2222222222222222222222222222222222222222"}`, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	for _, amount := range []string{"0", "501", "1.5", "9999999999999999999999999", "10junk"} {
		w := call("POST", fmt.Sprintf(`{"amount":%q,"from":"0x2222222222222222222222222222222222222222"}`, amount), true)
		if w.Code != 400 {
			t.Fatalf("invalid amount %s: %d", amount, w.Code)
		}
	}
	w := call("POST", `{"amount":"10","from":"0x2222222222222222222222222222222222222222"}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	payment = cryptoStore.payments[acc.ID]
	if payment.Credits != 1000 || !cryptoHash(payment.Nonce) {
		t.Fatal("bad payment", payment)
	}
	if w := call("POST", fmt.Sprintf(`{"nonce":%q,"signature":%q}`, "0x"+strings.Repeat("b", 64), "0x"+strings.Repeat("1", 130)), true); w.Code != 409 {
		t.Fatal("foreign nonce accepted")
	}
	other := &auth.Account{ID: acc.ID + "_other", Approved: true, Created: time.Now()}
	auth.SetAccountForTest(other)
	defer auth.RemoveAccountForTest(other.ID)
	otherSession, err := auth.CreateSession(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Logout(otherSession.Token)
	foreign := httptest.NewRequest("POST", "https://micro.example/account/crypto", strings.NewReader(fmt.Sprintf(`{"nonce":%q,"signature":%q}`, payment.Nonce, "0x"+strings.Repeat("1", 130))))
	foreign.AddCookie(&http.Cookie{Name: "session", Value: otherSession.Token})
	foreign.Header.Set("X-CSRF-Token", auth.CSRFToken(foreign))
	foreignResult := httptest.NewRecorder()
	CryptoHandler(foreignResult, foreign)
	if foreignResult.Code != 409 {
		t.Fatal("another account claimed this payment")
	}
	// Hold the worker while testing the handler and persisted signed terms.
	cryptoRunning.Store(acc.ID, true)
	w = call("POST", fmt.Sprintf(`{"nonce":%q,"signature":%q,"amount":"500","from":"0x3333333333333333333333333333333333333333"}`, payment.Nonce, "0x"+strings.Repeat("1", 130)), true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", `{"amount":"10","from":"0x2222222222222222222222222222222222222222"}`, true); w.Code != 409 {
		t.Fatal("replaced pending payment")
	}
	if strings.Contains(call("GET", "", false).Body.String(), "signature") {
		t.Fatal("status disclosed signature")
	}
	var saved map[string]cryptoPayment
	if err := data.LoadJSON("crypto_payments.json", &saved); err != nil {
		t.Fatal(err)
	}
	cryptoStore.payments = saved
	cryptoRunning.Delete(acc.ID)
	var group sync.WaitGroup
	for range 5 {
		group.Add(1)
		go func() { defer group.Done(); settleCrypto(acc.ID) }()
	}
	group.Wait()
	if cryptoStore.payments[acc.ID].Status != "paid" || broadcast.Load() != 1 {
		t.Fatalf("not settled once: %s / %d", cryptoStore.payments[acc.ID].Status, broadcast.Load())
	}
	// Simulate losing the facilitator response and retrying after restart. The
	// nonce/Transfer receipt recovers the same payment without another broadcast.
	payment.Status = "pending"
	payment.Signature = "0x" + strings.Repeat("1", 130)
	cryptoStore.payments[acc.ID] = payment
	onchain.Store(true)
	settleCrypto(acc.ID)
	if broadcast.Load() != 1 || cryptoStore.payments[acc.ID].Status != "paid" {
		t.Fatal("recovery rebroadcast or failed")
	}
	count, total := 0, 0
	for _, tr := range transactions[acc.ID] {
		if tr.Operation == "topup_usdc" {
			count++
			total += tr.Amount
		}
	}
	if count != 1 || total != 1000 {
		t.Fatalf("credited %d times, %d credits", count, total)
	}
}

func cryptoTransferFixture(p cryptoPayment) cryptoLog {
	return cryptoLog{Address: baseUSDC, Topics: []string{cryptoTopic("Transfer(address,address,uint256)"), cryptoAddressTopic(p.From), cryptoAddressTopic(p.Requirement.PayTo)}, Data: fmt.Sprintf("0x%064x", creditsAsUSDCAtomic(p.Credits))}
}

func TestCryptoReceiptRejectsWrongTransfer(t *testing.T) {
	acc, _ := cryptoFixture(t)
	p := cryptoPayment{Account: acc.ID, Created: strconv.FormatInt(acc.Created.UnixNano(), 10), Credits: 1000, From: "0x2222222222222222222222222222222222222222"}
	p.Requirement = *x402.TopUpRequirement(1000)
	for _, change := range []func(*cryptoLog){
		func(l *cryptoLog) { l.Address = "0x3333333333333333333333333333333333333333" },
		func(l *cryptoLog) { l.Topics[1] = cryptoAddressTopic(p.Requirement.PayTo) },
		func(l *cryptoLog) { l.Topics[2] = cryptoAddressTopic(p.From) },
		func(l *cryptoLog) { l.Data = fmt.Sprintf("0x%064x", 1) },
		func(l *cryptoLog) { l.Removed = true },
		func(l *cryptoLog) { l.Topics = l.Topics[:2] },
	} {
		l := cryptoTransferFixture(p)
		if !validCryptoTransfer(p, l) {
			t.Fatal("valid transfer rejected")
		}
		change(&l)
		if validCryptoTransfer(p, l) {
			t.Fatal("incorrect transfer accepted")
		}
	}
}

func TestCryptoExpiryRequiresFinalizedChain(t *testing.T) {
	acc, _ := cryptoFixture(t)
	oldHTTP := cryptoHTTP
	defer func() { cryptoHTTP = oldHTTP }()
	p := cryptoPayment{Account: acc.ID, Created: strconv.FormatInt(acc.Created.UnixNano(), 10), Credits: 1000, From: "0x2222222222222222222222222222222222222222", Nonce: "0x" + strings.Repeat("a", 64), Block: 100, Before: 1600, Status: "pending", Requirement: *x402.TopUpRequirement(1000)}
	cryptoStore.payments[acc.ID] = p
	finalized := uint64(200)
	chain := "0x2105"
	cryptoHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "eth_chainId":
			result = chain
		case "eth_blockNumber":
			result = "0x10000"
		case "eth_getBlockByNumber":
			var label string
			json.Unmarshal(req.Params[0], &label)
			n := finalized
			if label != "finalized" {
				n, _ = strconv.ParseUint(strings.TrimPrefix(label, "0x"), 16, 64)
			}
			result = map[string]string{"number": fmt.Sprintf("0x%x", n), "timestamp": fmt.Sprintf("0x%x", 1000+n*2)}
		case "eth_getLogs":
			var filter map[string]string
			json.Unmarshal(req.Params[0], &filter)
			if finalized > 300 && filter["toBlock"] != "0x12c" {
				t.Errorf("unbounded recovery: %s", filter["toBlock"])
			}
			result = []cryptoLog{}
		default:
			t.Fatalf("unexpected %s", req.Method)
		}
		return stripeResponse(map[string]any{"result": result}), nil
	})}
	// Wall-clock expiry cannot discard a transfer that is not finalized yet.
	settleCrypto(acc.ID)
	if cryptoStore.payments[acc.ID].Status != "pending" {
		t.Fatal("expired before finality")
	}
	chain = "0x1"
	finalized = 10000
	settleCrypto(acc.ID)
	if cryptoStore.payments[acc.ID].Status != "pending" {
		t.Fatal("trusted another chain")
	}
	chain = "0x2105"
	settleCrypto(acc.ID)
	if cryptoStore.payments[acc.ID].Status != "expired" {
		t.Fatal("did not release confirmed unpaid request")
	}
}
