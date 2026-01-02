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
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

var (
	StripeSecretKey        = os.Getenv("STRIPE_SECRET_KEY")
	StripePublishableKey   = os.Getenv("STRIPE_PUBLISHABLE_KEY")
	StripeWebhookSecret    = os.Getenv("STRIPE_WEBHOOK_SECRET")
	StripeMembershipPrice  = os.Getenv("STRIPE_MEMBERSHIP_PRICE") // Monthly subscription price ID
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

	// Check if user is member/admin
	var acc *auth.Account
	isMember := false
	isAdmin := false
	hasSubscription := false
	if a, err := auth.GetAccount(userID); err == nil {
		acc = a
		isMember = acc.Member
		isAdmin = acc.Admin
		hasSubscription = acc.StripeSubscriptionID != ""
	}

	var sb strings.Builder

	// Status section
	sb.WriteString(`<h2>Your Wallet</h2>`)

	if isMember || isAdmin {
		// Member/Admin status card
		sb.WriteString(`<div class="card" style="background: #f0fff4; border-color: #22c55e;">`)
		sb.WriteString(`<div style="text-align: center; padding: 20px;">`)
		if isAdmin {
			sb.WriteString(`<div style="font-size: 14px; color: #666; margin-bottom: 5px;">Status</div>`)
			sb.WriteString(`<div style="font-size: 32px; font-weight: bold; color: #22c55e;">Admin</div>`)
		} else {
			sb.WriteString(`<div style="font-size: 14px; color: #666; margin-bottom: 5px;">Status</div>`)
			sb.WriteString(`<div style="font-size: 32px; font-weight: bold; color: #22c55e;">Member</div>`)
		}
		sb.WriteString(`<div style="font-size: 14px; color: #666; margin-top: 10px;">Unlimited searches included</div>`)
		if hasSubscription && IsStripeConfigured() {
			sb.WriteString(`<p style="margin-top: 15px;"><a href="/wallet/manage">Manage subscription →</a></p>`)
		}
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	} else {
		// Credit balance card
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<div style="text-align: center; padding: 20px;">`)
		sb.WriteString(`<div style="font-size: 14px; color: #666; margin-bottom: 5px;">Credit Balance</div>`)
		sb.WriteString(fmt.Sprintf(`<div style="font-size: 32px; font-weight: bold;">%s</div>`, FormatCredits(wallet.Balance)))
		sb.WriteString(fmt.Sprintf(`<div style="font-size: 14px; color: #666;">%d credits</div>`, wallet.Balance))
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)

		// Daily quota section
		usedPct := float64(usage.Searches) / float64(FreeDailySearches) * 100
		if usedPct > 100 {
			usedPct = 100
		}

		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Daily Free Searches</h3>`)
		sb.WriteString(`<div style="background: #e5e5e5; height: 8px; border-radius: 4px; overflow: hidden; margin: 15px 0;">`)
		sb.WriteString(fmt.Sprintf(`<div style="background: #000; height: 100%%; width: %.0f%%; transition: width 0.3s;"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<p style="margin: 0;"><strong>%d of %d remaining today</strong></p>`, freeRemaining, FreeDailySearches))
		sb.WriteString(`<p style="font-size: 12px; color: #666; margin-top: 5px;">Resets at midnight UTC</p>`)
		sb.WriteString(`</div>`)

		// Membership upsell
		if IsStripeConfigured() && StripeMembershipPrice != "" {
			sb.WriteString(`<div class="card" style="background: #fafafa;">`)
			sb.WriteString(`<h3>Become a Member</h3>`)
			sb.WriteString(`<p>Get unlimited searches and support Mu's development.</p>`)
			sb.WriteString(`<ul style="margin: 15px 0; padding-left: 20px;">`)
			sb.WriteString(`<li>Unlimited news, video, and chat searches</li>`)
			sb.WriteString(`<li>Access to private messaging</li>`)
			sb.WriteString(`<li>Support independent development</li>`)
			sb.WriteString(`</ul>`)
			sb.WriteString(`<p><a href="/wallet/subscribe">Subscribe →</a></p>`)
			sb.WriteString(`</div>`)
		}

		// Topup section (only if Stripe configured)
		if IsStripeConfigured() {
			sb.WriteString(`<div class="card">`)
			sb.WriteString(`<h3>Top Up Credits</h3>`)
			sb.WriteString(`<p style="color: #666; font-size: 14px;">1 credit = 1p • Use credits when daily free searches are exhausted</p>`)
			sb.WriteString(`<div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 10px; margin-top: 15px;">`)

			for _, tier := range TopupTiers {
				bonusLabel := ""
				if tier.BonusPct > 0 {
					bonusLabel = fmt.Sprintf(`<span style="background: #22c55e; color: white; font-size: 10px; padding: 2px 6px; border-radius: 10px; margin-top: 5px; display: inline-block;">+%d%%</span>`, tier.BonusPct)
				}
				sb.WriteString(fmt.Sprintf(`<button onclick="topupCredits(%d)" style="display: flex; flex-direction: column; align-items: center; padding: 15px; background: white; border: 1px solid var(--card-border);">`, tier.Amount))
				sb.WriteString(fmt.Sprintf(`<span style="font-size: 20px; font-weight: bold;">£%.2f</span>`, float64(tier.Amount)/100))
				sb.WriteString(fmt.Sprintf(`<span style="font-size: 12px; color: #666;">%d credits</span>`, tier.Credits))
				sb.WriteString(bonusLabel)
				sb.WriteString(`</button>`)
			}

			sb.WriteString(`</div>`)
			sb.WriteString(`</div>`)
		}
	}

	// Pricing info
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Credit Costs</h3>`)
	sb.WriteString(`<table style="width: 100%; border-collapse: collapse;">`)
	sb.WriteString(`<tr style="border-bottom: 1px solid var(--divider);"><th style="text-align: left; padding: 10px 0;">Feature</th><th style="text-align: right; padding: 10px 0;">Cost</th></tr>`)
	sb.WriteString(fmt.Sprintf(`<tr style="border-bottom: 1px solid var(--divider);"><td style="padding: 10px 0;">News Search</td><td style="text-align: right; padding: 10px 0;">%d credit%s</td></tr>`, CostNewsSearch, pluralize(CostNewsSearch)))
	sb.WriteString(fmt.Sprintf(`<tr style="border-bottom: 1px solid var(--divider);"><td style="padding: 10px 0;">Video Search</td><td style="text-align: right; padding: 10px 0;">%d credit%s</td></tr>`, CostVideoSearch, pluralize(CostVideoSearch)))
	sb.WriteString(fmt.Sprintf(`<tr><td style="padding: 10px 0;">Chat AI Query</td><td style="text-align: right; padding: 10px 0;">%d credit%s</td></tr>`, CostChatQuery, pluralize(CostChatQuery)))
	sb.WriteString(`</table>`)
	sb.WriteString(`<p style="font-size: 12px; color: #666; margin-top: 15px; margin-bottom: 0;"><a href="/plans">View all plans</a> • Members get unlimited access</p>`)
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Transaction History</h3>`)
		sb.WriteString(`<table style="width: 100%; border-collapse: collapse; font-size: 14px;">`)
		sb.WriteString(`<tr style="border-bottom: 1px solid var(--divider);"><th style="text-align: left; padding: 10px 0;">Date</th><th style="text-align: left; padding: 10px 0;">Type</th><th style="text-align: right; padding: 10px 0;">Amount</th><th style="text-align: right; padding: 10px 0;">Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Type == TxTopup {
				typeLabel = "Top Up"
			}
			amountColor := "#dc2626"
			amountPrefix := "-"
			if tx.Amount > 0 {
				amountColor = "#22c55e"
				amountPrefix = "+"
			}
			sb.WriteString(fmt.Sprintf(`<tr style="border-bottom: 1px solid var(--divider);">
				<td style="padding: 10px 0;">%s</td>
				<td style="padding: 10px 0;">%s</td>
				<td style="text-align: right; padding: 10px 0; color: %s;">%s%d</td>
				<td style="text-align: right; padding: 10px 0;">%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountColor, amountPrefix, abs(tx.Amount), tx.Balance))
		}

		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	// Add JavaScript for topup
	sb.WriteString(`
<script>
async function topupCredits(amount) {
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
		alert('Failed to start checkout: ' + err.message);
	}
}
</script>`)

	return sb.String()
}

// QuotaExceededPage renders the quota exceeded message
func QuotaExceededPage(operation string, cost int) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card" style="max-width: 500px; margin: 50px auto; text-align: center;">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your 10 free searches for today.</p>`)
	sb.WriteString(`<h3 style="margin-top: 20px;">Options</h3>`)
	sb.WriteString(`<ul style="text-align: left; margin: 15px 0;">`)
	sb.WriteString(`<li style="margin: 10px 0;">Wait until midnight UTC for more free searches</li>`)
	sb.WriteString(fmt.Sprintf(`<li style="margin: 10px 0;"><a href="/wallet">Use credits</a> (%d credit%s for this search)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li style="margin: 10px 0;"><a href="/plans">View all plans</a></li>`)
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

	switch {
	case path == "/wallet" && r.Method == "GET":
		handleWalletPage(w, r)
	case path == "/wallet/topup" && r.Method == "POST":
		handleTopup(w, r)
	case path == "/wallet/subscribe" && r.Method == "GET":
		handleSubscribePage(w, r)
	case path == "/wallet/subscribe" && r.Method == "POST":
		handleSubscribe(w, r)
	case path == "/wallet/manage":
		handleManageSubscription(w, r)
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

func handleWalletPage(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	content := WalletPage(sess.Account)
	html := app.RenderHTMLForRequest("Wallet", "Manage your credits", content, r)
	w.Write([]byte(html))
}

func handleTopup(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Authentication required"})
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
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	// Check if this was a subscription or credit purchase
	sessionType := r.URL.Query().Get("type")
	
	var content string
	if sessionType == "subscription" {
		content = `<div class="card" style="max-width: 400px; margin: 50px auto; text-align: center; background: #f0fff4; border-color: #22c55e;">
	<h2 style="color: #22c55e;">✓ Welcome, Member!</h2>
	<p>Your membership is now active. Enjoy unlimited searches!</p>
	<p style="margin-top: 20px;"><a href="/wallet">View Wallet →</a></p>
</div>`
	} else {
		content = `<div class="card" style="max-width: 400px; margin: 50px auto; text-align: center; background: #f0fff4; border-color: #22c55e;">
	<h2 style="color: #22c55e;">✓ Payment Successful</h2>
	<p>Your credits have been added to your wallet.</p>
	<p style="margin-top: 20px;"><a href="/wallet">View Wallet →</a></p>
</div>`
	}

	html := app.RenderHTMLForRequest("Payment Successful", "Your payment was successful", content, r)
	w.Write([]byte(html))
	_ = sess // Used for potential future user-specific success messages
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

// handleSubscribePage shows the subscription checkout page
func handleSubscribePage(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", 404)
		return
	}

	// Already a member
	if acc.Member {
		http.Redirect(w, r, "/wallet", 302)
		return
	}

	// Check if Stripe subscriptions are configured
	if !IsStripeConfigured() || StripeMembershipPrice == "" {
		content := `<div class="card" style="max-width: 500px; margin: 50px auto; text-align: center;">
	<h2>Memberships Not Available</h2>
	<p>Stripe subscriptions are not configured for this instance.</p>
	<p style="margin-top: 20px;"><a href="/membership">View membership info →</a></p>
</div>`
		html := app.RenderHTMLForRequest("Subscribe", "Become a member", content, r)
		w.Write([]byte(html))
		return
	}

	content := `<div class="card" style="max-width: 500px; margin: 50px auto;">
	<h2>Become a Member</h2>
	<p>Get unlimited access to all features with a monthly subscription.</p>
	
	<h3 style="margin-top: 20px;">What's included:</h3>
	<ul style="margin: 15px 0; padding-left: 20px;">
		<li>Unlimited news searches</li>
		<li>Unlimited video searches</li>
		<li>Unlimited AI chat queries</li>
		<li>Access to private messaging</li>
		<li>Support independent development</li>
	</ul>
	
	<p style="margin-top: 20px; text-align: center;">
		<button onclick="startSubscription()" style="padding: 15px 40px; font-size: 16px;">Subscribe Now</button>
	</p>
	<p style="font-size: 12px; color: #666; text-align: center; margin-top: 10px;">Cancel anytime • Managed by Stripe</p>
</div>

<script>
async function startSubscription() {
	try {
		const resp = await fetch('/wallet/subscribe', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'}
		});
		const data = await resp.json();
		if (data.url) {
			window.location.href = data.url;
		} else if (data.error) {
			alert('Error: ' + data.error);
		}
	} catch (err) {
		alert('Failed to start checkout: ' + err.message);
	}
}
</script>`

	html := app.RenderHTMLForRequest("Subscribe", "Become a member", content, r)
	w.Write([]byte(html))
}

// handleSubscribe creates a Stripe subscription checkout session
func handleSubscribe(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Authentication required"})
		return
	}

	if !IsStripeConfigured() || StripeMembershipPrice == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "Subscriptions not configured"})
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "Account not found"})
		return
	}

	// Already a member
	if acc.Member {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Already a member"})
		return
	}

	// Build domain for redirect URLs
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "localhost") {
		scheme = "http"
	}
	domain := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Create checkout session for subscription
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Price:    stripe.String(StripeMembershipPrice),
			Quantity: stripe.Int64(1),
		}},
		SuccessURL:        stripe.String(domain + "/wallet/success?type=subscription&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String(domain + "/wallet/cancel"),
		ClientReferenceID: stripe.String(sess.Account),
	}

	// If user has existing Stripe customer, use it
	if acc.StripeCustomerID != "" {
		params.Customer = stripe.String(acc.StripeCustomerID)
	} else {
		params.CustomerEmail = stripe.String(sess.Account) // Use account ID as email placeholder
	}

	params.AddMetadata("user_id", sess.Account)
	params.AddMetadata("type", "subscription")

	checkoutSession, err := checkoutsession.New(params)
	if err != nil {
		app.Log("wallet", "Stripe subscription checkout error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create checkout session"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": checkoutSession.URL})
}

// handleManageSubscription redirects to Stripe billing portal
func handleManageSubscription(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", 302)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", 404)
		return
	}

	if acc.StripeCustomerID == "" {
		http.Redirect(w, r, "/wallet", 302)
		return
	}

	// Build domain for redirect URLs
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "localhost") {
		scheme = "http"
	}
	domain := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Create billing portal session
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(acc.StripeCustomerID),
		ReturnURL: stripe.String(domain + "/wallet"),
	}

	portalSession, err := portalsession.New(params)
	if err != nil {
		app.Log("wallet", "Failed to create billing portal session: %v", err)
		http.Error(w, "Failed to access billing portal", 500)
		return
	}

	http.Redirect(w, r, portalSession.URL, 302)
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
		sessionType := checkoutSession.Metadata["type"]

		if sessionType == "subscription" {
			// Handle subscription checkout completion
			handleSubscriptionCreated(userID, &checkoutSession)
		} else {
			// Handle credit top-up
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
		}

	case "customer.subscription.created", "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			app.Log("wallet", "Failed to parse subscription: %v", err)
			http.Error(w, "Invalid subscription data", 400)
			return
		}

		// Find user by customer ID
		userID := findUserByCustomerID(sub.Customer.ID)
		if userID == "" {
			app.Log("wallet", "No user found for customer %s", sub.Customer.ID)
			// Not an error - might be a subscription we don't track
			w.WriteHeader(200)
			return
		}

		// Update member status based on subscription status
		if sub.Status == stripe.SubscriptionStatusActive || sub.Status == stripe.SubscriptionStatusTrialing {
			setMemberStatus(userID, true, sub.Customer.ID, sub.ID)
			app.Log("wallet", "Activated membership for user %s (subscription: %s)", userID, sub.ID)
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			app.Log("wallet", "Failed to parse subscription: %v", err)
			http.Error(w, "Invalid subscription data", 400)
			return
		}

		// Find user by customer ID
		userID := findUserByCustomerID(sub.Customer.ID)
		if userID == "" {
			app.Log("wallet", "No user found for customer %s", sub.Customer.ID)
			w.WriteHeader(200)
			return
		}

		// Revoke member status
		setMemberStatus(userID, false, sub.Customer.ID, "")
		app.Log("wallet", "Revoked membership for user %s (subscription cancelled)", userID)

	case "invoice.payment_failed":
		app.Log("wallet", "Invoice payment failed: %s", string(event.Data.Raw))

	case "payment_intent.payment_failed":
		app.Log("wallet", "Payment failed: %s", string(event.Data.Raw))
	}

	w.WriteHeader(200)
}

// handleSubscriptionCreated processes a successful subscription checkout
func handleSubscriptionCreated(userID string, session *stripe.CheckoutSession) {
	if userID == "" {
		app.Log("wallet", "No user ID in subscription checkout")
		return
	}

	// Get the subscription details
	customerID := ""
	subscriptionID := ""
	
	if session.Customer != nil {
		customerID = session.Customer.ID
	}
	if session.Subscription != nil {
		subscriptionID = session.Subscription.ID
	}

	// Update user account
	setMemberStatus(userID, true, customerID, subscriptionID)
	app.Log("wallet", "Subscription created for user %s (customer: %s, subscription: %s)", userID, customerID, subscriptionID)
}

// findUserByCustomerID finds a user account by their Stripe customer ID
func findUserByCustomerID(customerID string) string {
	accounts := auth.GetAllAccounts()
	for _, acc := range accounts {
		if acc.StripeCustomerID == customerID {
			return acc.ID
		}
	}
	return ""
}

// setMemberStatus updates a user's member status and Stripe IDs
func setMemberStatus(userID string, isMember bool, customerID, subscriptionID string) {
	acc, err := auth.GetAccount(userID)
	if err != nil {
		app.Log("wallet", "Failed to get account %s: %v", userID, err)
		return
	}

	acc.Member = isMember
	if customerID != "" {
		acc.StripeCustomerID = customerID
	}
	acc.StripeSubscriptionID = subscriptionID

	if err := auth.UpdateAccount(acc); err != nil {
		app.Log("wallet", "Failed to update account %s: %v", userID, err)
	}
}

// IsStripeConfigured returns true if Stripe is properly configured
func IsStripeConfigured() bool {
	return StripeSecretKey != ""
}
