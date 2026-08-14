package wallet

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"mu/internal/quota"

	"mu/internal/app"
	"mu/internal/auth"
)

// cryptoWalletCard renders the user's Base (USDC) wallet on the /wallet page:
// balance, a tap-to-copy address and a fund QR.
//
// This is a way to top up credits, not a second currency to spend. Everything a
// caller is charged is in credits; USDC held here converts into them.
//
// Offered only when CryptoTopupEnabled. Paying in crypto is a thing to explain
// before a card is, and an instance not pursuing it should not put that in
// front of someone deciding how to pay.
//
// The one exception is a user who already holds USDC here. That is real money
// at a real address, and hiding the only screen that can move it would strand
// it — so they still get the card, worded as a way out rather than an
// invitation in.
func cryptoWalletCard(userID string) string {
	bw, err := GetOrCreateWallet(userID)
	// Say so rather than vanishing. A card that renders with an empty address
	// and a QR code of nothing is worse than no card: it looks like the feature
	// half-works, and the operator who just switched it on has nothing to go on.
	if err != nil || bw == nil || bw.Address == "" {
		if !CryptoTopupEnabled() {
			return ""
		}
		return `<div class="card"><h3>Top up with USDC</h3>` +
			`<p class="text-sm text-muted">This instance offers crypto top-up, but your ` +
			`wallet could not be opened, so there is no address to show. The server log ` +
			`under <code>wallet</code> says why.</p></div>`
	}
	usdc, raw := USDCBalance(bw.Address)
	holding := raw != nil && raw.Sign() > 0
	if !CryptoTopupEnabled() && !holding {
		return ""
	}

	heading, blurb := "Top up with USDC", `Send <b>USDC on Base</b> to this address and convert it into credits.`
	if !CryptoTopupEnabled() {
		heading = "Your USDC balance"
		blurb = `This instance tops up by card. You hold USDC at the address below — convert it into credits, or move it out.`
	}

	return fmt.Sprintf(`<div class="card">
  <h3>`+heading+`</h3>
  <p class="text-sm text-muted">`+blurb+`</p>
  <p style="font-size:24px;margin:6px 0 10px"><b>$%s</b> <span style="color:#999;font-size:14px">USDC</span></p>
  <button type="button" class="cw-convert" onclick="cwConvert(this)">Convert to credits →</button>
  <p class="text-sm text-muted" style="margin:6px 0 12px">Moves your USDC into your credit balance (1 USDC = 100 credits), gas-free.</p>
  <button type="button" class="cw-addr" data-addr="%s" onclick="cwCopy(this)">%s</button>
  <div class="cw-copied" id="cw-copied" hidden>Copied to clipboard ✓</div>
  <details class="cw-qrwrap"><summary>Show QR code</summary><div class="cw-qr" id="cw-qr"></div></details>
</div>
<style>
.cw-addr{display:block;width:100%%;text-align:left;font-family:ui-monospace,Menlo,monospace;font-size:13px;word-break:break-all;background:#f5f5f5;padding:11px;border:1px solid #e2e2e2;border-radius:6px;color:#222;cursor:pointer}
.cw-addr:hover{background:var(--hover-background,#f5f5f5);border-color:var(--border-color,#ddd)}
.cw-copied{font-size:12px;color:#1a7f37;margin-top:6px}
.cw-qrwrap{margin-top:10px;font-size:13px;color:#666}
.cw-qrwrap summary{cursor:pointer}
.cw-qr{margin-top:8px}.cw-qr img{width:180px;height:180px;image-rendering:pixelated}
.cw-convert{padding:9px 16px;font-size:14px;font-weight:600;border:0;border-radius:6px;background:#1a7f37;color:#fff;cursor:pointer}
.cw-convert:hover{background:#166b2e}
.cw-convert[disabled]{opacity:.6;cursor:default}
</style>
<script src="/qrcode.js"></script>
<script>
(function(){var addr=document.querySelector('.cw-addr');if(!addr)return;var a=addr.getAttribute('data-addr');
var q=document.getElementById('cw-qr');if(q&&window.qrcode){try{var qr=qrcode(0,'M');qr.addData(a);qr.make();q.innerHTML=qr.createImgTag(4,8);}catch(e){}}})();
function cwCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function cwCopy(el){var a=el.getAttribute('data-addr');function done(){var c=document.getElementById('cw-copied');if(c){c.hidden=false;setTimeout(function(){c.hidden=true;},1800);}}
  if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(a).then(done).catch(function(){cwFallback(a,done);});}else{cwFallback(a,done);}}
function cwFallback(a,done){var t=document.createElement('textarea');t.value=a;t.style.position='fixed';t.style.opacity='0';document.body.appendChild(t);t.select();try{document.execCommand('copy');done();}catch(e){}document.body.removeChild(t);}
function cwConvert(el){el.disabled=true;var t=el.textContent;el.textContent='Converting…';
  fetch('/wallet/convert',{method:'POST',headers:{'X-CSRF-Token':cwCsrf()}}).then(function(r){return r.json();}).then(function(d){
    if(d.error){alert(d.error);el.disabled=false;el.textContent=t;return;}
    location.reload();}).catch(function(){el.disabled=false;el.textContent=t;});}
</script>`, usdc, htmlEsc(bw.Address), htmlEsc(bw.Address))
}

// htmlEsc escapes text for HTML.
//
// It delegates rather than reimplementing, because this package had its own
// version and it escaped one character fewer than the others: & < > and the
// double quote, but not the single quote. Nothing here puts output in a
// single-quoted attribute today, so it was a hazard rather than a hole — and
// the next single-quoted attribute would have made it one, with nothing to say
// the escaper was weaker than it looked.
func htmlEsc(s string) string { return html.EscapeString(s) }

// money renders pence as £20. Plans are whole pounds; one that is not would be
// a pricing decision rather than a formatting one.
func money(pence int) string {
	if pence%100 == 0 {
		return fmt.Sprintf("£%d", pence/100)
	}
	return fmt.Sprintf("£%.2f", float64(pence)/100)
}

// thousands renders 2000 as 2,000.
func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// WalletPage renders the wallet page HTML
func WalletPage(userID string) string {
	wallet := GetWallet(userID)
	transactions := GetTransactions(userID, 20)

	// Check if user is admin
	isAdmin := false
	if acc, err := auth.GetAccount(userID); err == nil {
		isAdmin = acc.Admin
	}

	var sb strings.Builder

	// Balance
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Balance</h3>`)
	if isAdmin {
		sb.WriteString(`<p class="text-sm text-muted">Admin · Unlimited access</p>`)
		if wallet.Balance > 0 {
			sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
		}
	} else {
		sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
	}
	sb.WriteString(`<p><a href="/wallet/topup">Add Credits →</a> · <a href="/wallet/transfer">Transfer →</a></p>`)
	sb.WriteString(`</div>`)

	// USDC on Base. Only when this instance offers crypto top-up, or when the
	// user already holds some — see cryptoWalletCard.
	sb.WriteString(cryptoWalletCard(userID))

	// App earnings summary
	var totalEarnings int
	for _, tx := range transactions {
		if tx.Operation == quota.OpAppRevenue {
			totalEarnings += tx.Amount
		}
	}
	if totalEarnings > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>App Earnings</h3>`)
		sb.WriteString(fmt.Sprintf(`<p>%d credits earned from your apps (recent)</p>`, totalEarnings))
		sb.WriteString(`<p class="text-sm text-muted">You keep 90%% of every sale. <a href="/apps">Manage your apps →</a></p>`)
		sb.WriteString(`</div>`)
	}

	if !isAdmin {
		// Self-hosting note
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Self-Host</h3>`)
		sb.WriteString(`<p class="text-sm text-muted">Want unlimited? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
		sb.WriteString(`</div>`)
	}

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(PricingTableHTML())
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>History</h3>`)
		sb.WriteString(`<table class="data-table">`)
		sb.WriteString(`<tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Operation == quota.OpAppUse {
				if appSlug, ok := tx.Metadata["app"].(string); ok {
					typeLabel = "App: " + appSlug
				} else {
					typeLabel = "App usage"
				}
			} else if tx.Operation == quota.OpAppRevenue {
				if appSlug, ok := tx.Metadata["app"].(string); ok {
					typeLabel = "Earned: " + appSlug
				} else {
					typeLabel = "App revenue"
				}
			} else if tx.Type == TxTopup {
				typeLabel = "Deposit"
			} else if tx.Type == TxTransfer {
				// Prefer the name recorded with the transfer, then the name of
				// the id it went to, then the bare id. Receipts written before
				// names were recorded still resolve, and one whose account has
				// since gone still says something.
				who := func(nameKey, idKey string) string {
					if n, ok := tx.Metadata[nameKey].(string); ok && n != "" {
						return n
					}
					if id, ok := tx.Metadata[idKey].(string); ok && id != "" {
						return AccountLabel(id)
					}
					return ""
				}
				if tx.Amount > 0 {
					if from := who("from_name", "from"); from != "" {
						typeLabel = "Transfer from " + from
					} else {
						typeLabel = "Transfer in"
					}
				} else {
					if to := who("to_name", "to"); to != "" {
						typeLabel = "Transfer to " + to
					} else {
						typeLabel = "Transfer out"
					}
				}
			}
			var amountStr string
			if tx.Amount == 0 {
				amountStr = "included"
			} else if tx.Amount > 0 {
				amountStr = fmt.Sprintf("+%d", tx.Amount)
			} else {
				amountStr = fmt.Sprintf("-%d", abs(tx.Amount))
			}
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountStr, tx.Balance))
		}

		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

// QuotaExceededPage renders the quota exceeded message
func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Handler handles wallet-related HTTP requests
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// The balance, as data.
	//
	// This used to answer only to ?balance=1, so a caller that asked for JSON
	// and didn't know the flag got the rendered wallet page instead — 20KB of
	// HTML returned to an agent that called a tool named wallet_balance. The
	// tool dispatcher sets Accept: application/json on every path-backed call,
	// so honouring Accept fixes it here and for anything else routed this way.
	if r.URL.Query().Get("balance") == "1" || app.WantsJSON(r) {
		sess, _ := auth.TrySession(r)
		if sess == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		balance := GetBalance(sess.Account)
		app.RespondJSON(w, map[string]int{"balance": balance})
		return
	}

	switch {
	case path == "/wallet" && r.Method == "GET":
		handleWalletPage(w, r)
	case path == "/wallet/topup" && r.Method == "GET" && app.WantsJSON(r):
		handleTopupJSON(w, r)
	case path == "/wallet/topup" && r.Method == "GET":
		handleDepositPage(w, r)
	case path == "/wallet/stripe/checkout" && r.Method == "POST":
		handleStripeCheckout(w, r)
	case path == "/wallet/stripe/success" && r.Method == "GET":
		handleStripeSuccess(w, r)
	case path == "/wallet/stripe/webhook" && r.Method == "POST":
		HandleStripeWebhook(w, r)
	case path == "/wallet/convert" && r.Method == "POST":
		handleConvert(w, r)
	case path == "/wallet/transfer" && r.Method == "POST":
		handleTransfer(w, r)
	case path == "/wallet/transfer" && r.Method == "GET":
		handleTransferPage(w, r)
	case path == "/wallet/pricing":
		handlePricing(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleConvert sweeps the user's USDC balance to the treasury and credits
// their account (USDC → credits), then returns the new credit balance.
func handleConvert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess, _ := auth.TrySession(r)
	if sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"login required"}`))
		return
	}
	credited, err := ConvertUSDCToCredits(sess.Account)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"credited": credited, "balance": GetBalance(sess.Account)})
}

func handleWalletPage(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)

	var content string
	if sess != nil {
		content = WalletPage(sess.Account)
	} else {
		content = PublicWalletPage()
	}

	html := app.RenderHTMLForRequest("Wallet", "Credits and pricing", content, r)
	w.Write([]byte(html))
}

// PublicWalletPage renders the wallet page for unauthenticated users
func PublicWalletPage() string {
	var sb strings.Builder

	// Intro
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Credits &amp; Pricing</h3>`)
	sb.WriteString(`<p>Browsing is included. AI and search features use credits. Top up and pay as you go — no subscription required.</p>`)
	sb.WriteString(`<p><a href="/login" class="btn">Login to view your balance</a>&nbsp;<a href="/signup" class="btn btn-secondary">Sign up</a></p>`)
	sb.WriteString(`</div>`)

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(PricingTableHTML())
	sb.WriteString(`</div>`)

	// Topup options
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Top Up</h3>`)
	sb.WriteString(`<p>Add credits to your account via card:</p>`)
	sb.WriteString(`<ul>`)
	if StripeEnabled() {
		sb.WriteString(`<li><strong>Card</strong> — secure payment via Stripe</li>`)
	}
	sb.WriteString(`</ul>`)
	sb.WriteString(`<p><a href="/login">Login</a> or <a href="/signup">sign up</a> to top up.</p>`)
	sb.WriteString(`</div>`)

	// This used to carry a second card offering pay-per-request in stablecoin.
	// Credits are the one thing a caller pays in — an agent's calls and a
	// person's draw on the same balance — so there is nothing to compare here.

	// Self-hosting note
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Self-Host</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">Want unlimited? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

func handleDepositPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	var sb strings.Builder

	if StripeEnabled() {
		sb.WriteString(renderStripeDeposit(sess.Account, r.URL.Query().Get("error")))
	} else {
		sb.WriteString(`<div class="card"><p class="text-error">No payment methods available.</p></div>`)
	}

	html := app.RenderHTMLForRequest("Add Credits", "Top up your wallet", sb.String(), r)
	w.Write([]byte(html))
}

func renderStripeDeposit(userID, errMsg string) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}
	sb.WriteString(`<hr style="border:none;border-top:1px solid #eee;margin:16px 0">`)

	sb.WriteString("<h4>One-time top-up</h4>")
	sb.WriteString(`<form method="POST" action="/wallet/stripe/checkout">`)

	// Preset quick-select buttons
	sb.WriteString(`<div class="d-flex gap-2 mb-3 mt-2">`)
	for _, tier := range StripeTopupTiers {
		sb.WriteString(fmt.Sprintf(
			`<button type="button" class="btn btn-secondary" onclick="document.getElementById('topup-amount').value='%d'">%s</button>`,
			tier.Amount/100, tier.Label))
	}
	sb.WriteString(`</div>`)

	// Custom amount input (in whole pounds)
	sb.WriteString(`<div>`)
	sb.WriteString(`<label for="topup-amount" class="text-sm">Amount (£)</label>`)
	sb.WriteString(fmt.Sprintf(`<input type="number" id="topup-amount" name="amount" min="1" max="%d" placeholder="e.g. 10" required class="form-input w-full mt-1">`, maxTopupPounds))
	sb.WriteString(`</div>`)

	sb.WriteString(`<button type="submit" class="btn mt-4">Continue to Payment</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<p class="text-sm text-muted">Secure payment via Stripe. 1 credit = 1p.</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

// maxTransferCredits is the maximum allowed transfer amount in credits
const maxTransferCredits = 50000 // £500

func handleTransferPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	balance := GetBalance(sess.Account)
	errMsg := r.URL.Query().Get("error")
	successMsg := r.URL.Query().Get("success")

	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Transfer Credits</h3>`)
	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}
	if successMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-success">%s</p>`, successMsg))
	}
	sb.WriteString(fmt.Sprintf(`<p>Your balance: <strong>%d credits</strong></p>`, balance))
	remaining := DailyTransferCap - DailyTransferTotal(sess.Account, time.Now())
	if remaining < 0 {
		remaining = 0
	}
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">Daily transfer limit: %d credits. Remaining today: %d credits.</p>`, DailyTransferCap, remaining))
	// Autocomplete offers usernames, because that is what the transfer resolves
	// and what the field asks for. It offered display names, which are free
	// text and not unique — so picking a suggestion could fill in a word that
	// belonged to somebody else's account, which is how 100 credits went to a
	// stranger who happened to have typed the recipient's handle into their
	// profile. The display name rides along as the label, since that is how you
	// recognise a person.
	allAccounts := auth.GetAllAccounts()
	sb.WriteString(`<datalist id="user-list">`)
	for _, a := range allAccounts {
		if a.ID == sess.Account {
			continue
		}
		label := a.ID
		if n := strings.TrimSpace(a.Name); n != "" && !strings.EqualFold(n, a.ID) {
			label = a.ID + " — " + n
		}
		sb.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`,
			htmlEsc(a.ID), htmlEsc(label)))
	}
	sb.WriteString(`</datalist>`)

	sb.WriteString(`<form method="POST" action="/wallet/transfer">`)
	sb.WriteString(`<div>`)
	sb.WriteString(`<label for="transfer-to" class="text-sm">Recipient</label>`)
	sb.WriteString(`<input type="text" id="transfer-to" name="to" placeholder="username" required class="form-input w-full mt-1" list="user-list" autocomplete="off">`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="transfer-amount" class="text-sm">Amount (credits)</label>`)
	sb.WriteString(fmt.Sprintf(`<input type="number" id="transfer-amount" name="amount" min="1" max="%d" placeholder="e.g. 100" required class="form-input w-full mt-1">`, maxTransferCredits))
	sb.WriteString(`</div>`)
	sb.WriteString(`<button type="submit" class="btn mt-4">Transfer</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">1 credit = 1p. Transfers are instant and non-reversible. Daily transfer limit: %d credits.</p>`, DailyTransferCap))
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Transfer Credits", "Send credits to another user", sb.String(), r)
	w.Write([]byte(html))
}

func handleTransfer(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) || app.SendsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		app.RedirectToLogin(w, r)
		return
	}

	var to string
	var amount int

	if app.SendsJSON(r) {
		// JSON body
		var body struct {
			To     string `json:"to"`
			Amount int    `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			app.RespondJSON(w, map[string]string{"error": "invalid request body"})
			return
		}
		to = body.To
		amount = body.Amount
	} else {
		// Form submission
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/wallet/transfer?error=Invalid+form", http.StatusSeeOther)
			return
		}
		to = r.FormValue("to")
		fmt.Sscanf(r.FormValue("amount"), "%d", &amount)
	}

	to = strings.TrimSpace(to)
	to = strings.TrimPrefix(to, "@")

	if to == "" {
		respondTransferError(w, r, "Recipient username is required")
		return
	}
	if amount < 1 {
		respondTransferError(w, r, "Amount must be at least 1 credit")
		return
	}
	if amount > maxTransferCredits {
		respondTransferError(w, r, fmt.Sprintf("Maximum transfer is %d credits", maxTransferCredits))
		return
	}

	// By username, which is the account id — never by display name. A display
	// name is free text and not unique, so resolving one would let anybody
	// receive a transfer meant for somebody else by typing their handle into a
	// profile field.
	recipient, err := auth.AccountByUsername(to)
	if err != nil {
		respondTransferError(w, r, "No account with the username "+to)
		return
	}

	if recipient.ID == sess.Account {
		respondTransferError(w, r, "Cannot transfer to yourself")
		return
	}

	// Perform the transfer
	if err := TransferCredits(sess.Account, recipient.ID, amount); err != nil {
		respondTransferError(w, r, err.Error())
		return
	}

	if app.WantsJSON(r) || app.SendsJSON(r) {
		newBalance := GetBalance(sess.Account)
		app.RespondJSON(w, map[string]interface{}{
			"status":  "ok",
			"to":      recipient.Name,
			"amount":  amount,
			"balance": newBalance,
		})
		return
	}

	msg := fmt.Sprintf("Transferred %d credits to %s", amount, recipient.Name)
	http.Redirect(w, r, "/wallet/transfer?success="+neturl.QueryEscape(msg), http.StatusSeeOther)
}

func respondTransferError(w http.ResponseWriter, r *http.Request, msg string) {
	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, map[string]string{"error": msg})
		return
	}
	http.Redirect(w, r, "/wallet/transfer?error="+neturl.QueryEscape(msg), http.StatusSeeOther)
}

// maxTopupPounds is the maximum allowed top-up amount in whole pounds
const maxTopupPounds = 500

type TopupMethod struct {
	Type  string            `json:"type"`            // "card"
	Tiers []StripeTopupTier `json:"tiers,omitempty"` // For card/Stripe
}

func handleTopupJSON(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.RespondJSON(w, map[string]string{"error": "authentication required"})
		return
	}

	var methods []TopupMethod

	if StripeEnabled() {
		methods = append(methods, TopupMethod{
			Type:  "card",
			Tiers: StripeTopupTiers,
		})
	}

	app.RespondJSON(w, map[string]interface{}{
		"methods": methods,
	})
}

func handleStripeCheckout(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/wallet/topup?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}

	// Amount is submitted in whole pounds; convert to pence for Stripe
	amountStr := r.FormValue("amount")
	var pounds int
	fmt.Sscanf(amountStr, "%d", &pounds)

	if pounds < 1 {
		http.Redirect(w, r, "/wallet/topup?error=Please+enter+an+amount", http.StatusSeeOther)
		return
	}
	if pounds > maxTopupPounds {
		http.Redirect(w, r, fmt.Sprintf("/wallet/topup?error=Maximum+top-up+is+%%C2%%A3%d", maxTopupPounds), http.StatusSeeOther)
		return
	}

	amount := pounds * 100 // convert to pence

	// Success/cancel URLs must name the public origin — see app.BaseURL, which
	// is the single answer to "what is this instance's address".
	baseURL := app.BaseURL(r)
	successURL := baseURL + "/wallet/stripe/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := baseURL + "/wallet/topup"

	// Create checkout session
	checkoutURL, err := CreateCheckoutSession(sess.Account, amount, successURL, cancelURL)
	if err != nil {
		app.Log("stripe", "checkout error: %v", err)
		content := `<div class="card"><h2>Payment Error</h2><p>Failed to create checkout session. Please try again.</p><p><a href="/wallet/topup" class="btn">Back</a></p></div>`
		html := app.RenderHTMLForRequest("Payment Error", "Checkout failed", content, r)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(html))
		return
	}

	// Redirect to Stripe
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

// handleStripeSuccess is where Stripe returns somebody after they have paid,
// and it settles the purchase rather than only announcing it.
//
// It used to say "your credits will be added shortly" and do nothing, because
// the webhook did the work. A webhook is a promise from a service you do not
// control: if it is not configured, or its secret is wrong, or its event list
// is missing the one that matters, the card is charged and nothing happens here
// — no credits, no plan, no error, and nothing to find out from except an
// account asking why its balance is zero. Which is how this was found.
//
// Both routes call the same function and it runs once per session id, so
// whichever arrives first wins and the other is free.
func handleStripeSuccess(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	account := ""
	if sess != nil {
		account = sess.Account
	}

	settled := false
	if id := r.URL.Query().Get("session_id"); id != "" && account != "" {
		if err := SettleCheckout(id, account); err != nil {
			app.Log("stripe", "settling %s on return: %v", id, err)
		} else {
			settled = true
		}
	}

	body := `<p>Your credits will be added to your account shortly.</p>`
	if settled {
		body = fmt.Sprintf(`<p>Your balance is now <strong>%d credits</strong>.</p>`,
			GetBalance(account))
	}
	content := `<div class="card">
		<h2>Payment complete</h2>` + body + `
		<p><a href="/wallet" class="btn">View wallet</a></p>
	</div>`
	html := app.RenderHTMLForRequest("Payment complete", "Credits added", content, r)
	w.Write([]byte(html))
}

// PricingItem is one billable operation and what it costs.
type PricingItem struct {
	Operation   string `json:"operation"`
	Description string `json:"description"`
	Cost        int    `json:"cost"`
	Unit        string `json:"unit"`
}

// pricingItem is the pre-export name, kept as an alias so existing internal
// references keep compiling.
type pricingItem = PricingItem

// Pricing returns every billable operation, cheapest first. This is the single
// source of truth for what things cost: the wallet page, the signed-out wallet
// page, the /wallet/pricing API and the public pricing page all render from it.
// They each used to carry their own hardcoded table, which drifted — image
// generation was the most expensive op a user could trigger and three of the
// four tables omitted it entirely.
//
// Anything added to the Cost* vars belongs here too.
// Pricing is what this instance charges, for the cost tables on /wallet, the
// signed-out wallet page, the pricing API and /pricing.
//
// It reads internal/quota's list rather than keeping its own. There used to be
// two: a switch of thirty constants in one package and a hand-written table of
// labels in this one, in different orders, and they drifted — image generation,
// the most expensive thing a user could trigger short of building an app, was
// missing from three of the four tables that rendered from here.
func Pricing() []PricingItem {
	out := make([]PricingItem, 0)
	for _, p := range quota.Prices() {
		out = append(out, PricingItem{
			Operation:   p.Op,
			Description: p.Label,
			Cost:        p.Cost,
			Unit:        "credits",
		})
	}
	return out
}

func getPricingData() []PricingItem { return Pricing() }

// PricingTableHTML renders the shared cost table. Costs are whole pence
// (1 credit = 1p), so the same figures serve both the credit and the currency
// framing.
func PricingTableHTML() string {
	var free []string
	var charged []PricingItem
	for _, it := range Pricing() {
		if it.Cost == 0 {
			free = append(free, it.Description)
			continue
		}
		charged = append(charged, it)
	}

	var sb strings.Builder
	sb.WriteString(`<table class="stats-table">`)
	// The price column never wraps. The first column holds a joined list of
	// every free operation, which is long enough to squeeze this one until the
	// browser broke the word — the costs table read "include" on one line and
	// "d" on the next.
	const price = `<td style="white-space:nowrap">`
	sb.WriteString(`<tr><td>Reading news, blogs, videos, markets, weather</td>` + price + `included</td></tr>`)
	// Zero-cost operations are listed as included rather than as "0p" rows —
	// a price of zero is not a price.
	if len(free) > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td>`+price+`included</td></tr>`,
			htmlEsc(strings.Join(free, ", "))))
	}
	for _, it := range charged {
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td>`+price+`%dp</td></tr>`,
			htmlEsc(it.Description), it.Cost))
	}
	// Paid apps are charged per request at a price the app's author sets, so
	// there is no fixed figure to list — but the mechanism exists (see
	// ChargeAppUse) and a cost table that omits it is not the source of truth
	// it claims to be.
	sb.WriteString(`<tr><td>Using a paid app</td><td>set by its author</td></tr>`)
	sb.WriteString(`</table>`)
	sb.WriteString(`<p class="text-sm text-muted">Most apps are free. Paid ones show their price before you run them; the author keeps 90%.</p>`)
	return sb.String()
}

func handlePricing(w http.ResponseWriter, r *http.Request) {
	items := getPricingData()

	if app.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"currency":     "GBP",
			"credit_value": "£0.01",
			"operations":   items,
		})
		return
	}

	var sb strings.Builder
	sb.WriteString(`<div class="max-w-xl"><div class="card">`)
	sb.WriteString(`<h3>Pricing</h3>`)
	sb.WriteString(`<p class="info">1 credit = £0.01. Browsing included. AI and search use credits.</p>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(`<tr><td>News, blogs, videos</td><td>included</td></tr>`)
	for _, item := range items {
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, item.Description, item.Cost))
	}
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)

	// App marketplace pricing
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>App Marketplace</h3>`)
	sb.WriteString(`<p class="info">Build apps and charge per use. You set the price, you earn the revenue.</p>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(`<tr><td>Price range</td><td>0–1000 credits per use</td></tr>`)
	sb.WriteString(`<tr><td>Creator share</td><td>90%</td></tr>`)
	sb.WriteString(`<tr><td>Platform fee</td><td>10%</td></tr>`)
	sb.WriteString(`<tr><td>Free apps</td><td>No charge</td></tr>`)
	sb.WriteString(`</table>`)
	sb.WriteString(`<p class="info mt-3"><a href="/apps/new">Build an app →</a></p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<p class="info mt-3">JSON: <code>curl -H "Accept: application/json" /wallet/pricing</code></p>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Pricing", "Platform pricing and costs", sb.String(), r)
	w.Write([]byte(html))
}
