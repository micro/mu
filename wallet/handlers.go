package wallet

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// WalletPage renders the wallet page HTML
func WalletPage(userID string) string {
	wallet := GetWallet(userID)
	usage := GetDailyUsage(userID)
	freeRemaining := GetFreeQuotaRemaining(userID)
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
	sb.WriteString(`<p><a href="/wallet/topup">Add Credits →</a></p>`)
	sb.WriteString(`</div>`)

	if !isAdmin {
		// Daily quota
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Free Queries</h3>`)
		usedPct := float64(usage.Used) / float64(FreeDailyQuota) * 100
		if usedPct > 100 {
			usedPct = 100
		}
		sb.WriteString(`<div class="progress">`)
		sb.WriteString(fmt.Sprintf(`<div class="progress-bar" style="width: %.0f%%;"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%d of %d remaining · Resets midnight UTC</p>`, freeRemaining, FreeDailyQuota))
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
	sb.WriteString(`<tr><td>News, blogs, videos</td><td>free</td></tr>`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td>%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td>%dp</td></tr>`, CostVideoSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Blog post</td><td>%dp</td></tr>`, CostBlogCreate))
	sb.WriteString(fmt.Sprintf(`<tr><td>Discussion post</td><td>%dp</td></tr>`, CostSocialPost))
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td>%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>Agent (standard)</td><td>%dp</td></tr>`, CostAgentQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>Agent (premium)</td><td>%dp</td></tr>`, CostAgentQueryPremium))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places search</td><td>%dp</td></tr>`, CostPlacesSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places nearby</td><td>%dp</td></tr>`, CostPlacesNearby))
	sb.WriteString(fmt.Sprintf(`<tr><td>Send mail</td><td>%dp</td></tr>`, CostMailSend))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(fmt.Sprintf(`<tr><td>Web search</td><td>%dp</td></tr>`, CostWebSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Web fetch</td><td>%dp</td></tr>`, CostWebFetch))
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
			var amountStr string
			if tx.Amount == 0 {
				amountStr = "free"
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
func QuotaExceededPage(operation string, cost int) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card center-card-md">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your free queries for today.</p>`)
	sb.WriteString(`<h3 class="mt-5">Options</h3>`)
	sb.WriteString(`<ul class="options-list">`)
	sb.WriteString(`<li>Wait until midnight UTC for more free queries</li>`)
	sb.WriteString(fmt.Sprintf(`<li><a href="/wallet">Use credits</a> (%d credit%s for this)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li><a href="/wallet/topup">Add credits</a></li>`)
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
	default:
		http.NotFound(w, r)
	}
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
	sb.WriteString(`<p>Mu is free with ` + fmt.Sprintf("%d", FreeDailyQuota) + ` queries/day. Need more? Top up and pay as you go — no subscription required.</p>`)
	sb.WriteString(`<p><a href="/login" class="btn">Login to view your balance</a>&nbsp;<a href="/signup" class="btn btn-secondary">Sign up free</a></p>`)
	sb.WriteString(`</div>`)

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(`<tr><td>News, blogs, videos</td><td>free</td></tr>`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td>%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td>%dp</td></tr>`, CostVideoSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Blog post</td><td>%dp</td></tr>`, CostBlogCreate))
	sb.WriteString(fmt.Sprintf(`<tr><td>Discussion post</td><td>%dp</td></tr>`, CostSocialPost))
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td>%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places search</td><td>%dp</td></tr>`, CostPlacesSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places nearby</td><td>%dp</td></tr>`, CostPlacesNearby))
	sb.WriteString(fmt.Sprintf(`<tr><td>Send mail</td><td>%dp</td></tr>`, CostMailSend))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(fmt.Sprintf(`<tr><td>Web search</td><td>%dp</td></tr>`, CostWebSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Web fetch</td><td>%dp</td></tr>`, CostWebFetch))
	sb.WriteString(`</table>`)
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

	// Self-hosting note
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Self-Host</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">Want unlimited and free? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
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
	sb.WriteString("<span>Topup via card</span>")
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

	// Build success/cancel URLs, preferring explicit config then proxy headers.
	// NOTE: X-Forwarded-Host should only be trusted when running behind a
	// reverse proxy that strips/sets this header (not passed through from clients).
	var baseURL string
	if domain := os.Getenv("MU_DOMAIN"); domain != "" {
		domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
		baseURL = "https://" + domain
	} else if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		fwdHost = strings.TrimPrefix(strings.TrimPrefix(fwdHost, "https://"), "http://")
		baseURL = "https://" + fwdHost
	} else {
		scheme := "https"
		if r.TLS == nil && !strings.Contains(r.Host, "mu.xyz") {
			scheme = "http"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
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
