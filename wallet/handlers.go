package wallet

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/skip2/go-qrcode"

	"mu/app"
	"mu/auth"
)

// WalletPage renders the wallet page HTML
func WalletPage(userID string) string {
	wallet := GetWallet(userID)
	usage := GetDailyUsage(userID)
	freeRemaining := GetFreeSearchesRemaining(userID)
	transactions := GetTransactions(userID, 20)

	// Check if user is admin
	isAdmin := false
	if acc, err := auth.GetAccount(userID); err == nil {
		isAdmin = acc.Admin
	}

	var sb strings.Builder

	if isAdmin {
		// Admin status
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Status</h3>`)
		sb.WriteString(`<p>Admin · Full access</p>`)
		sb.WriteString(`</div>`)
	} else {
		// Balance
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Balance</h3>`)
		sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
		sb.WriteString(`<p><a href="/wallet/deposit">Add Credits →</a></p>`)
		sb.WriteString(`</div>`)

		// Daily quota
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Free Queries</h3>`)
		usedPct := float64(usage.Searches) / float64(FreeDailySearches) * 100
		if usedPct > 100 {
			usedPct = 100
		}
		sb.WriteString(`<div class="progress">`)
		sb.WriteString(fmt.Sprintf(`<div class="progress-bar" style="width: %.0f%%;"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%d of %d remaining · Resets midnight UTC</p>`, freeRemaining, FreeDailySearches))
		sb.WriteString(`</div>`)

		// Self-hosting note
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Self-Host</h3>`)
		sb.WriteString(`<p class="text-sm text-muted">Want unlimited and free? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
		sb.WriteString(`</div>`)
	}

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td>%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>News summary</td><td>%dp</td></tr>`, CostNewsSummary))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td>%dp</td></tr>`, CostVideoSearch))
	if CostVideoWatch > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Video watch</td><td>%dp</td></tr>`, CostVideoWatch))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td>%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(fmt.Sprintf(`<tr><td>App create</td><td>%dp</td></tr>`, CostAppCreate))
	sb.WriteString(fmt.Sprintf(`<tr><td>App modify</td><td>%dp</td></tr>`, CostAppModify))
	sb.WriteString(fmt.Sprintf(`<tr><td>Agent run</td><td>%dp</td></tr>`, CostAgentRun))
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>History</h3>`)
		sb.WriteString(`<table class="data-table">`)
		sb.WriteString(`<tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Type == TxTopup {
				typeLabel = "Deposit"
			}
			amountPrefix := "-"
			if tx.Amount > 0 {
				amountPrefix = "+"
			}
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s%d</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountPrefix, abs(tx.Amount), tx.Balance))
		}

		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

// QuotaExceededPage renders the quota exceeded message
func QuotaExceededPage(operation string, cost int) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card center-card-md">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your free queries for today.</p>`)
	sb.WriteString(`<h3 class="mt-5">Options</h3>`)
	sb.WriteString(`<ul class="options-list">`)
	sb.WriteString(`<li>Wait until midnight UTC for more free queries</li>`)
	sb.WriteString(fmt.Sprintf(`<li><a href="/wallet">Use credits</a> (%d credit%s for this)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li><a href="/wallet/deposit">Add credits</a></li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

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

	// Check for balance JSON endpoint
	if r.URL.Query().Get("balance") == "1" {
		sess, _ := auth.TrySession(r)
		if sess == nil {
			app.RespondJSON(w, map[string]int{"balance": 0})
			return
		}
		balance := GetBalance(sess.Account)
		app.RespondJSON(w, map[string]int{"balance": balance})
		return
	}

	switch {
	case path == "/wallet" && r.Method == "GET":
		handleWalletPage(w, r)
	case path == "/wallet/deposit" && r.Method == "GET":
		handleDepositPage(w, r)
	case path == "/wallet/stripe/checkout" && r.Method == "POST":
		handleStripeCheckout(w, r)
	case path == "/wallet/stripe/success" && r.Method == "GET":
		handleStripeSuccess(w, r)
	case path == "/wallet/stripe/webhook" && r.Method == "POST":
		HandleStripeWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleWalletPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	content := WalletPage(sess.Account)
	html := app.RenderHTMLForRequest("Wallet", "Manage your credits", content, r)
	w.Write([]byte(html))
}

// Supported chains for deposits
var depositChains = []struct {
	ID      string
	Name    string
	ChainID int
}{
	{"ethereum", "Ethereum", 1},
	{"base", "Base", 8453},
	{"arbitrum", "Arbitrum", 42161},
	{"optimism", "Optimism", 10},
}

func handleDepositPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	// Get selected method (stripe or crypto)
	method := r.URL.Query().Get("method")
	if method == "" {
		method = "stripe" // Default to stripe
	}

	var sb strings.Builder

	// Method tabs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Topup using card or crypto</h3>`)
	sb.WriteString(`<div class="d-flex gap-2">`)
	if StripeEnabled() {
		stripeActive := ""
		if method == "stripe" {
			stripeActive = " btn-primary"
		} else {
			stripeActive = " btn-secondary"
		}
		sb.WriteString(fmt.Sprintf(`<a href="/wallet/deposit?method=stripe" class="btn%s">Card</a>`, stripeActive))
	}
	if CryptoWalletEnabled() {
		cryptoActive := ""
		if method == "crypto" {
			cryptoActive = " btn-primary"
		} else {
			cryptoActive = " btn-secondary"
		}
		sb.WriteString(fmt.Sprintf(`<a href="/wallet/deposit?method=crypto" class="btn%s">Crypto</a>`, cryptoActive))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	if method == "stripe" && StripeEnabled() {
		sb.WriteString(renderStripeDeposit())
	} else if method == "crypto" && CryptoWalletEnabled() {
		sb.WriteString(renderCryptoDeposit(sess.Account, r))
	} else if StripeEnabled() {
		sb.WriteString(renderStripeDeposit())
	} else if CryptoWalletEnabled() {
		sb.WriteString(renderCryptoDeposit(sess.Account, r))
	} else {
		sb.WriteString(`<div class="card"><p class="text-error">No payment methods available.</p></div>`)
	}

	html := app.RenderHTMLForRequest("Add Credits", "Top up your wallet", sb.String(), r)
	w.Write([]byte(html))
}

func renderStripeDeposit() string {
	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Select Amount</h3>`)
	sb.WriteString(`<form method="POST" action="/wallet/stripe/checkout">`)
	sb.WriteString(`<div class="d-flex flex-column gap-2">`)

	for _, tier := range StripeTopupTiers {
		bonusText := ""
		if tier.BonusPct > 0 {
			bonusText = fmt.Sprintf(" <span class=\"text-success\">+%d%% bonus</span>", tier.BonusPct)
		}
		sb.WriteString(fmt.Sprintf(`<label class="topup-option">`+
			`<input type="radio" name="amount" value="%d"> `+
			`<strong>%s</strong> → %d credits%s`+
			`</label>`, tier.Amount, tier.Label, tier.Credits, bonusText))
	}

	sb.WriteString(`</div>`)
	sb.WriteString(`<button type="submit" class="btn mt-4">Continue to Payment</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<p class="text-sm text-muted">Secure payment via Stripe. 1 credit = 1p.</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

func renderCryptoDeposit(userID string, r *http.Request) string {
	// Initialize crypto wallet if needed
	if err := InitCryptoWallet(); err != nil {
		app.Log("wallet", "Failed to init crypto wallet: %v", err)
		return `<div class="card"><p class="text-error">Crypto wallet not available. Please try again later.</p></div>`
	}

	// Get selected chain (default to ethereum)
	selectedChain := r.URL.Query().Get("chain")
	if selectedChain == "" {
		selectedChain = "ethereum"
	}

	// Find chain info
	chainID := 1 // default ethereum
	chainName := "Ethereum"
	for _, c := range depositChains {
		if c.ID == selectedChain {
			chainID = c.ChainID
			chainName = c.Name
			break
		}
	}

	// Get user's deposit address
	depositAddr, err := GetUserDepositAddress(userID)
	if err != nil {
		app.Log("wallet", "Failed to get deposit address: %v", err)
		return `<div class="card"><p class="text-error">Failed to generate deposit address.</p></div>`
	}

	var sb strings.Builder

	// Chain selector
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Pick a network</h3>`)
	sb.WriteString(`<select id="chain-select" onchange="window.location.href='/wallet/deposit?method=crypto&chain='+this.value" style="padding: 8px; border-radius: 4px; border: 1px solid #ddd;">`)
	for _, c := range depositChains {
		selected := ""
		if c.ID == selectedChain {
			selected = " selected"
		}
		sb.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, c.ID, selected, c.Name))
	}
	sb.WriteString(`</select>`)
	sb.WriteString(`</div>`)

	// Generate QR code with ethereum: URI
	ethURI := fmt.Sprintf("ethereum:%s@%d", depositAddr, chainID)
	qrPNG, _ := qrcode.Encode(ethURI, qrcode.Medium, 200)
	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	// Deposit address with QR
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Deposit Address</h3>`)
	sb.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">%s</p>`, chainName))
	sb.WriteString(fmt.Sprintf(`<img src="data:image/png;base64,%s" alt="QR Code" class="qr-code">`, qrBase64))
	sb.WriteString(fmt.Sprintf(`<code class="deposit-address">%s</code>`, depositAddr))
	sb.WriteString(`<p class="text-sm mt-3">`)
	sb.WriteString(`<button onclick="navigator.clipboard.writeText('` + depositAddr + `'); this.textContent='Copied!'; setTimeout(() => this.textContent='Copy', 2000)" class="btn-secondary">Copy</button>`)
	sb.WriteString(fmt.Sprintf(` <a href="%s" class="btn ml-2">Open Wallet</a>`, ethURI))
	sb.WriteString(`</p>`)
	sb.WriteString(`</div>`)

	// Conversion note
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<p class="text-sm text-muted">1 credit = 1p · Converted at market rate</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

func handleStripeCheckout(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	amountStr := r.FormValue("amount")
	var amount int
	fmt.Sscanf(amountStr, "%d", &amount)

	if amount == 0 {
		http.Error(w, "please select an amount", http.StatusBadRequest)
		return
	}

	// Build success/cancel URLs
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "mu.xyz") {
		scheme = "http"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	successURL := baseURL + "/wallet/stripe/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := baseURL + "/wallet/deposit?method=stripe"

	// Create checkout session
	checkoutURL, err := CreateCheckoutSession(sess.Account, amount, successURL, cancelURL)
	if err != nil {
		app.Log("stripe", "checkout error: %v", err)
		http.Error(w, "failed to create checkout session", http.StatusInternalServerError)
		return
	}

	// Redirect to Stripe
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

func handleStripeSuccess(w http.ResponseWriter, r *http.Request) {
	// Just show success message - actual crediting happens via webhook
	content := `<div class="card">
		<h2>Payment Successful</h2>
		<p>Your credits will be added to your account shortly.</p>
		<p><a href="/wallet" class="btn">View Wallet</a></p>
	</div>`
	html := app.RenderHTMLForRequest("Payment Complete", "Credits added", content, r)
	w.Write([]byte(html))
}
