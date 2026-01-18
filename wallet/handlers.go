package wallet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"mu/app"
	"mu/auth"

	"github.com/stripe/stripe-go/v76"
	checkoutsession "github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

var (
	StripeSecretKey      = os.Getenv("STRIPE_SECRET_KEY")
	StripePublishableKey = os.Getenv("STRIPE_PUBLISHABLE_KEY")
	StripeWebhookSecret  = os.Getenv("STRIPE_WEBHOOK_SECRET")
)

// Price IDs from environment (set up in Stripe Dashboard)
var stripePrices = map[int]string{
	500:  os.Getenv("STRIPE_PRICE_500"),
	1000: os.Getenv("STRIPE_PRICE_1000"),
	2500: os.Getenv("STRIPE_PRICE_2500"),
	5000: os.Getenv("STRIPE_PRICE_5000"),
}

func init() {
	if StripeSecretKey != "" {
		stripe.Key = StripeSecretKey
	}
}

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
		if IsStripeConfigured() {
			sb.WriteString(`<p><a href="/wallet/topup">Top up →</a></p>`)
		}
		sb.WriteString(`</div>`)

		// Daily quota
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Free Queries</h3>`)
		usedPct := float64(usage.Searches) / float64(FreeDailySearches) * 100
		if usedPct > 100 {
			usedPct = 100
		}
		sb.WriteString(`<div style="background: #eee; height: 6px; border-radius: 3px; margin: 10px 0;">`)
		sb.WriteString(fmt.Sprintf(`<div style="background: #000; height: 100%%; width: %.0f%%; border-radius: 3px;"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<p style="font-size: 14px; color: #666;">%d of %d remaining · Resets midnight UTC</p>`, freeRemaining, FreeDailySearches))
		sb.WriteString(`</div>`)

		// Self-hosting note
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Self-Host</h3>`)
		sb.WriteString(`<p style="font-size: 14px; color: #666;">Want unlimited and free? <a href="https://github.com/asim/mu">Self-host your own instance</a>.</p>`)
		sb.WriteString(`</div>`)
	}

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(`<table style="width: 100%; font-size: 14px;">`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td style="text-align: right;">%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>News summary</td><td style="text-align: right;">%dp</td></tr>`, CostNewsSummary))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td style="text-align: right;">%dp</td></tr>`, CostVideoSearch))
	if CostVideoWatch > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Video watch</td><td style="text-align: right;">%dp</td></tr>`, CostVideoWatch))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td style="text-align: right;">%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td style="text-align: right;">%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>History</h3>`)
		sb.WriteString(`<table style="width: 100%; border-collapse: collapse; font-size: 14px;">`)
		sb.WriteString(`<tr style="border-bottom: 1px solid #eee;"><th style="text-align: left; padding: 10px 0;">Date</th><th style="text-align: left; padding: 10px 0;">Type</th><th style="text-align: right; padding: 10px 0;">Amount</th><th style="text-align: right; padding: 10px 0;">Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Type == TxTopup {
				typeLabel = "Top Up"
			}
			amountPrefix := "-"
			if tx.Amount > 0 {
				amountPrefix = "+"
			}
			sb.WriteString(fmt.Sprintf(`<tr style="border-bottom: 1px solid #eee;">
				<td style="padding: 10px 0;">%s</td>
				<td style="padding: 10px 0;">%s</td>
				<td style="text-align: right; padding: 10px 0;">%s%d</td>
				<td style="text-align: right; padding: 10px 0;">%d</td>
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

	sb.WriteString(`<div class="card" style="max-width: 500px; margin: 50px auto; text-align: center;">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your free queries for today.</p>`)
	sb.WriteString(`<h3 style="margin-top: 20px;">Options</h3>`)
	sb.WriteString(`<ul style="text-align: left; margin: 15px 0;">`)
	sb.WriteString(`<li style="margin: 10px 0;">Wait until midnight UTC for more free queries</li>`)
	sb.WriteString(fmt.Sprintf(`<li style="margin: 10px 0;"><a href="/wallet">Use credits</a> (%d credit%s for this)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li style="margin: 10px 0;"><a href="/plans">View pricing</a></li>`)
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
	case path == "/wallet/topup" && r.Method == "GET":
		handleTopupPage(w, r)
	case path == "/wallet/topup" && r.Method == "POST":
		handleTopup(w, r)
	case path == "/wallet/success":
		handleSuccess(w, r)
	case path == "/wallet/cancel":
		handleCancel(w, r)
	case path == "/wallet/webhook" && r.Method == "POST":
		handleWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleTopupPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if !IsStripeConfigured() {
		http.Redirect(w, r, "/wallet", 302)
		return
	}

	var sb strings.Builder

	sb.WriteString(`<h3>Top Up</h3>`)
	sb.WriteString(`<p style="font-size: 14px; color: #666;">1 credit = 1p</p>`)

	for _, tier := range TopupTiers {
		bonus := ""
		if tier.BonusPct > 0 {
			bonus = fmt.Sprintf(" (+%d%%)", tier.BonusPct)
		}
		sb.WriteString(fmt.Sprintf(`<p><a href="#" onclick="topup(%d); return false;">£%d → %d credits%s</a></p>`,
			tier.Amount, tier.Amount/100, tier.Credits, bonus))
	}

	sb.WriteString(`
<script>
async function topup(amount) {
	try {
		const resp = await fetch('/wallet/topup', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({amount: amount})
		});
		const data = await resp.json();
		if (data.url) {
			window.location.href = data.url;
		} else if (data.error) {
			alert('Error: ' + data.error);
		}
	} catch (err) {
		alert('Failed: ' + err.message);
	}
}
</script>`)

	html := app.RenderHTMLForRequest("Top Up", "Add credits", sb.String(), r)
	_ = sess
	w.Write([]byte(html))
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

func handleTopup(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RespondJSON(w, map[string]string{"error": "Authentication required"})
		return
	}

	if StripeSecretKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "Payment system not configured"})
		return
	}

	var req struct {
		Amount int `json:"amount"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	tier := GetTopupTier(req.Amount)
	if tier == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid amount"})
		return
	}

	// Get price ID from environment or use dynamic pricing
	priceID := stripePrices[req.Amount]

	// Build domain for redirect URLs
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "localhost") {
		scheme = "http"
	}
	domain := fmt.Sprintf("%s://%s", scheme, r.Host)

	var params *stripe.CheckoutSessionParams

	if priceID != "" {
		// Use pre-configured Stripe Price
		params = &stripe.CheckoutSessionParams{
			Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
			LineItems: []*stripe.CheckoutSessionLineItemParams{{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			}},
			SuccessURL:        stripe.String(domain + "/wallet/success?session_id={CHECKOUT_SESSION_ID}"),
			CancelURL:         stripe.String(domain + "/wallet/cancel"),
			ClientReferenceID: stripe.String(sess.Account),
		}
	} else {
		// Use dynamic pricing (create price on the fly)
		params = &stripe.CheckoutSessionParams{
			Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
			LineItems: []*stripe.CheckoutSessionLineItemParams{{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String("gbp"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(fmt.Sprintf("%d Credits", tier.Credits)),
						Description: stripe.String(fmt.Sprintf("Top up your Mu wallet with %d credits", tier.Credits)),
					},
					UnitAmount: stripe.Int64(int64(tier.Amount)),
				},
				Quantity: stripe.Int64(1),
			}},
			SuccessURL:        stripe.String(domain + "/wallet/success?session_id={CHECKOUT_SESSION_ID}"),
			CancelURL:         stripe.String(domain + "/wallet/cancel"),
			ClientReferenceID: stripe.String(sess.Account),
		}
	}

	params.AddMetadata("user_id", sess.Account)
	params.AddMetadata("credits", strconv.Itoa(tier.Credits))
	params.AddMetadata("amount", strconv.Itoa(tier.Amount))

	checkoutSession, err := checkoutsession.New(params)
	if err != nil {
		app.Log("wallet", "Stripe checkout error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create checkout session"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": checkoutSession.URL})
}

func handleSuccess(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	content := `<div class="card" style="max-width: 400px; margin: 50px auto; text-align: center; background: #f0fff4; border-color: #22c55e;">
	<h2 style="color: #22c55e;">✓ Payment Successful</h2>
	<p>Your credits have been added to your wallet.</p>
	<p style="margin-top: 20px;"><a href="/wallet">View Wallet →</a></p>
</div>`

	html := app.RenderHTMLForRequest("Payment Successful", "Your payment was successful", content, r)
	w.Write([]byte(html))
	_ = sess
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	content := `<div class="card" style="max-width: 400px; margin: 50px auto; text-align: center; background: #fffbeb; border-color: #f59e0b;">
	<h2 style="color: #d97706;">Payment Cancelled</h2>
	<p>Your payment was cancelled. No charges were made.</p>
	<p style="margin-top: 20px;"><a href="/wallet">Back to Wallet →</a></p>
</div>`

	html := app.RenderHTMLForRequest("Payment Cancelled", "Your payment was cancelled", content, r)
	w.Write([]byte(html))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", 400)
		return
	}

	var event stripe.Event

	if StripeWebhookSecret != "" {
		// Verify webhook signature
		sig := r.Header.Get("Stripe-Signature")
		event, err = webhook.ConstructEvent(payload, sig, StripeWebhookSecret)
		if err != nil {
			app.Log("wallet", "Webhook signature verification failed: %v", err)
			http.Error(w, "Invalid signature", 400)
			return
		}
	} else {
		// Development mode - parse without verification
		if err := json.Unmarshal(payload, &event); err != nil {
			http.Error(w, "Invalid payload", 400)
			return
		}
	}

	switch event.Type {
	case "checkout.session.completed":
		var checkoutSession stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &checkoutSession); err != nil {
			app.Log("wallet", "Failed to parse checkout session: %v", err)
			http.Error(w, "Invalid session data", 400)
			return
		}

		userID := checkoutSession.ClientReferenceID
		creditsStr := checkoutSession.Metadata["credits"]
		credits, _ := strconv.Atoi(creditsStr)

		if userID == "" || credits == 0 {
			app.Log("wallet", "Invalid webhook data: userID=%s credits=%d", userID, credits)
			http.Error(w, "Invalid metadata", 400)
			return
		}

		// Add credits to wallet
		err := AddCredits(userID, credits, OpTopup, map[string]interface{}{
			"stripe_session_id": checkoutSession.ID,
			"payment_intent":    checkoutSession.PaymentIntent,
			"amount_total":      checkoutSession.AmountTotal,
		})
		if err != nil {
			app.Log("wallet", "Failed to add credits: %v", err)
			http.Error(w, "Failed to add credits", 500)
			return
		}

		app.Log("wallet", "Added %d credits to user %s (session: %s)", credits, userID, checkoutSession.ID)

	case "payment_intent.payment_failed":
		app.Log("wallet", "Payment failed: %s", string(event.Data.Raw))
	}

	w.WriteHeader(200)
}

// IsStripeConfigured returns true if Stripe is properly configured
func IsStripeConfigured() bool {
	return StripeSecretKey != ""
}
