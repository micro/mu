package account

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

// BalanceCard is what an account has to spend, at the top of /account.
//
// It sits directly under the profile because it is the one thing on that page
// with a deadline: everything else — a display name, a language, a passkey —
// can wait, and an empty balance stops the agent mid-errand. It used to be a
// nav item of its own called Wallet, which put a person's money one click
// further away than their choice of language.
func BalanceCard(userID string) string {
	c := CreditsOf(userID)

	isAdmin := false
	if acc, err := auth.GetAccount(userID); err == nil {
		isAdmin = acc.Admin
	}

	var sb strings.Builder
	sb.WriteString(`<div class="card" id="balance">`)
	sb.WriteString(`<h4>Balance</h4>`)
	sb.WriteString(fmt.Sprintf(`<p style="font-size:28px;margin:4px 0 2px"><b>%s</b> `+
		`<span style="color:#999;font-size:14px">credits</span></p>`, thousands(c.Balance)))
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted" style="margin:0 0 10px">%s · 1 credit = 1p</p>`,
		money(c.Balance)))
	if isAdmin {
		sb.WriteString(`<p class="text-sm text-muted" style="margin:0 0 10px">` +
			`You are an admin on this instance, so your own calls are never charged.</p>`)
	}
	sb.WriteString(`<p style="margin:0"><a href="/account/topup">Add credits →</a> · ` +
		`<a href="/account/transfer">Transfer →</a> · <a href="#ledger">History →</a></p>`)
	sb.WriteString(`</div>`)
	return sb.String()
}

// LedgerSection is the receipts: what things cost, and what this account has
// actually been charged.
//
// Below the fold on purpose. Somebody arriving at /account wants the number and
// the button; the itemised history is what they scroll to when the number is
// not what they expected.
func LedgerSection(userID string) string {
	transactions := Transactions(userID, 20)

	var sb strings.Builder

	// App earnings, when there are any. Money coming in reads differently from
	// money going out and should not be a row in the same table.
	var totalEarnings int
	for _, tx := range transactions {
		if tx.Operation == quota.OpAppRevenue {
			totalEarnings += tx.Amount
		}
	}
	if totalEarnings > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h4>App earnings</h4>`)
		sb.WriteString(fmt.Sprintf(`<p>%d credits earned from your apps (recent)</p>`, totalEarnings))
		sb.WriteString(`<p class="text-sm text-muted">You keep 90%% of every sale. <a href="/apps">Manage your apps →</a></p>`)
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`<div class="card" id="ledger">`)
	sb.WriteString(`<h4>Costs</h4>`)
	sb.WriteString(PricingTableHTML())
	sb.WriteString(`</div>`)

	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h4>History</h4>`)
		sb.WriteString(`<table class="data-table">`)
		sb.WriteString(`<tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th></tr>`)
		for _, tx := range transactions {
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), htmlEsc(transactionLabel(tx)),
				transactionAmount(tx), tx.Balance))
		}
		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

// transactionLabel names a movement in words somebody recognises.
func transactionLabel(tx *Transaction) string {
	switch {
	case tx.Operation == quota.OpAppUse:
		if appSlug, ok := tx.Metadata["app"].(string); ok {
			return "App: " + appSlug
		}
		return "App usage"
	case tx.Operation == quota.OpAppRevenue:
		if appSlug, ok := tx.Metadata["app"].(string); ok {
			return "Earned: " + appSlug
		}
		return "App revenue"
	case tx.Type == TxTopup:
		return "Deposit"
	case tx.Type == TxTransfer:
		// Prefer the name recorded with the transfer, then the name of the id it
		// went to, then the bare id. Receipts written before names were recorded
		// still resolve, and one whose account has since gone still says
		// something.
		who := func(nameKey, idKey string) string {
			if n, ok := tx.Metadata[nameKey].(string); ok && n != "" {
				return n
			}
			if id, ok := tx.Metadata[idKey].(string); ok && id != "" {
				return Label(id)
			}
			return ""
		}
		if tx.Amount > 0 {
			if from := who("from_name", "from"); from != "" {
				return "Transfer from " + from
			}
			return "Transfer in"
		}
		if to := who("to_name", "to"); to != "" {
			return "Transfer to " + to
		}
		return "Transfer out"
	}
	return tx.Operation
}

// transactionAmount renders a movement, including the zero that means a call
// was free rather than uncharged.
func transactionAmount(tx *Transaction) string {
	switch {
	case tx.Amount == 0:
		return "included"
	case tx.Amount > 0:
		return fmt.Sprintf("+%d", tx.Amount)
	}
	return fmt.Sprintf("-%d", abs(tx.Amount))
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

// BalanceHandler serves everything under /account that involves money.
//
// The paths were /wallet/* and moved with the ledger; the old ones redirect.
//
// The Stripe webhook is not among them and is not routed through here at all.
// It is a contract with somebody outside this process rather than a page, so it
// is registered on its own at /stripe/webhook — see routes.go.
func BalanceHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// The balance, as data.
	//
	// This used to answer only to ?balance=1, so a caller that asked for JSON
	// and didn't know the flag got a rendered page instead — 20KB of HTML
	// returned to something that only wanted a number. The tool dispatcher sets
	// Accept: application/json on every path-backed call, so honouring Accept
	// fixes it here and for anything else routed this way.
	if r.URL.Query().Get("balance") == "1" || app.WantsJSON(r) {
		sess, _ := auth.TrySession(r)
		if sess == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		balance := Balance(sess.Account)
		app.RespondJSON(w, map[string]int{"balance": balance})
		return
	}

	switch {
	case path == "/account/topup" && r.Method == "GET" && app.WantsJSON(r):
		handleTopupJSON(w, r)
	case path == "/account/topup" && r.Method == "GET":
		handleDepositPage(w, r)
	case path == "/account/stripe/checkout" && r.Method == "POST":
		handleStripeCheckout(w, r)
	case path == "/account/stripe/success" && r.Method == "GET":
		handleStripeSuccess(w, r)
	case path == "/account/transfer" && r.Method == "POST":
		handleTransfer(w, r)
	case path == "/account/transfer" && r.Method == "GET":
		handleTransferPage(w, r)
	case path == "/account/pricing":
		handlePricing(w, r)
	default:
		http.NotFound(w, r)
	}
}

// MovedToAccount sends the old money URLs to the page that now holds them.
//
// /wallet is a service now — a key an agent spends — so the old paths do not
// merely change, they come to mean something else. A link somebody bookmarked
// for their balance has to land on their balance rather than on a crypto
// address that happens to live at the same prefix.
func MovedToAccount(w http.ResponseWriter, r *http.Request) {
	to := "/account"
	switch strings.TrimSuffix(r.URL.Path, "/") {
	case "/wallet/topup":
		to = "/account/topup"
	case "/wallet/transfer":
		to = "/account/transfer"
	case "/wallet/pricing":
		to = "/account/pricing"
	case "/wallet/stripe/checkout":
		to = "/account/stripe/checkout"
	case "/wallet/stripe/success":
		to = "/account/stripe/success"
	}
	if q := r.URL.RawQuery; q != "" {
		to += "?" + q
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
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
	sb.WriteString(`<form method="POST" action="/account/stripe/checkout">`)

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

	balance := Balance(sess.Account)
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

	sb.WriteString(`<form method="POST" action="/account/transfer">`)
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
			http.Redirect(w, r, "/account/transfer?error=Invalid+form", http.StatusSeeOther)
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
		newBalance := Balance(sess.Account)
		app.RespondJSON(w, map[string]interface{}{
			"status":  "ok",
			"to":      recipient.Name,
			"amount":  amount,
			"balance": newBalance,
		})
		return
	}

	msg := fmt.Sprintf("Transferred %d credits to %s", amount, recipient.Name)
	http.Redirect(w, r, "/account/transfer?success="+neturl.QueryEscape(msg), http.StatusSeeOther)
}

func respondTransferError(w http.ResponseWriter, r *http.Request, msg string) {
	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, map[string]string{"error": msg})
		return
	}
	http.Redirect(w, r, "/account/transfer?error="+neturl.QueryEscape(msg), http.StatusSeeOther)
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
		http.Redirect(w, r, "/account/topup?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}

	// Amount is submitted in whole pounds; convert to pence for Stripe
	amountStr := r.FormValue("amount")
	var pounds int
	fmt.Sscanf(amountStr, "%d", &pounds)

	if pounds < 1 {
		http.Redirect(w, r, "/account/topup?error=Please+enter+an+amount", http.StatusSeeOther)
		return
	}
	if pounds > maxTopupPounds {
		http.Redirect(w, r, fmt.Sprintf("/account/topup?error=Maximum+top-up+is+%%C2%%A3%d", maxTopupPounds), http.StatusSeeOther)
		return
	}

	amount := pounds * 100 // convert to pence

	// Success/cancel URLs must name the public origin — see app.BaseURL, which
	// is the single answer to "what is this instance's address".
	baseURL := app.BaseURL(r)
	successURL := baseURL + "/account/stripe/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := baseURL + "/account/topup"

	// Create checkout session
	checkoutURL, err := CreateCheckoutSession(sess.Account, amount, successURL, cancelURL)
	if err != nil {
		app.Log("stripe", "checkout error: %v", err)
		content := `<div class="card"><h2>Payment Error</h2><p>Failed to create checkout session. Please try again.</p><p><a href="/account/topup" class="btn">Back</a></p></div>`
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
			Balance(account))
	}
	content := `<div class="card">
		<h2>Payment complete</h2>` + body + `
		<p><a href="/account" class="btn">View your balance</a></p>
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
// page, the /account/pricing API and the public pricing page all render from it.
// They each used to carry their own hardcoded table, which drifted — image
// generation was the most expensive op a user could trigger and three of the
// four tables omitted it entirely.
//
// Anything added to the Cost* vars belongs here too.
// Pricing is what this instance charges, for the cost tables on /account, the
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

	sb.WriteString(`<p class="info mt-3">JSON: <code>curl -H "Accept: application/json" /account/pricing</code></p>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Pricing", "Platform pricing and costs", sb.String(), r)
	w.Write([]byte(html))
}
