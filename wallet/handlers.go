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
	"github.com/stripe/stripe-go/v76/checkout/session"
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

	// Check if user is member/admin
	isMember := false
	isAdmin := false
	if acc, err := auth.GetAccount(userID); err == nil {
		isMember = acc.Member
		isAdmin = acc.Admin
	}

	var sb strings.Builder

	// Balance section
	sb.WriteString(`<div class="wallet-container">`)
	sb.WriteString(`<h2>Your Wallet</h2>`)

	if isMember || isAdmin {
		sb.WriteString(`<div class="wallet-balance unlimited">`)
		sb.WriteString(`<div class="balance-label">Status</div>`)
		if isAdmin {
			sb.WriteString(`<div class="balance-amount">Admin</div>`)
		} else {
			sb.WriteString(`<div class="balance-amount">Member</div>`)
		}
		sb.WriteString(`<div class="balance-note">Unlimited searches included</div>`)
		sb.WriteString(`</div>`)
	} else {
		sb.WriteString(`<div class="wallet-balance">`)
		sb.WriteString(`<div class="balance-label">Credit Balance</div>`)
		sb.WriteString(fmt.Sprintf(`<div class="balance-amount">%s</div>`, FormatCredits(wallet.Balance)))
		sb.WriteString(fmt.Sprintf(`<div class="balance-credits">%d credits</div>`, wallet.Balance))
		sb.WriteString(`</div>`)

		// Daily quota section
		usedPct := float64(usage.Searches) / float64(FreeDailySearches) * 100
		if usedPct > 100 {
			usedPct = 100
		}

		sb.WriteString(`<div class="daily-quota">`)
		sb.WriteString(`<h3>Daily Free Searches</h3>`)
		sb.WriteString(`<div class="quota-bar">`)
		sb.WriteString(fmt.Sprintf(`<div class="quota-used" style="width: %.0f%%"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<div class="quota-text">%d of %d remaining today</div>`, freeRemaining, FreeDailySearches))
		sb.WriteString(`<div class="quota-note">Resets at midnight UTC</div>`)
		sb.WriteString(`</div>`)

		// Topup section
		sb.WriteString(`<div class="topup-section">`)
		sb.WriteString(`<h3>Top Up Credits</h3>`)
		sb.WriteString(`<p class="topup-info">1 credit = 1p • Use credits when daily free searches are exhausted</p>`)
		sb.WriteString(`<div class="topup-options">`)

		for _, tier := range TopupTiers {
			bonusLabel := ""
			if tier.BonusPct > 0 {
				bonusLabel = fmt.Sprintf(`<span class="bonus">+%d%% bonus</span>`, tier.BonusPct)
			}
			sb.WriteString(fmt.Sprintf(`<button class="topup-btn" onclick="topupCredits(%d)">`, tier.Amount))
			sb.WriteString(fmt.Sprintf(`<span class="price">£%.2f</span>`, float64(tier.Amount)/100))
			sb.WriteString(fmt.Sprintf(`<span class="credits">%d credits</span>`, tier.Credits))
			sb.WriteString(bonusLabel)
			sb.WriteString(`</button>`)
		}

		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Pricing info
	sb.WriteString(`<div class="pricing-info">`)
	sb.WriteString(`<h3>Credit Costs</h3>`)
	sb.WriteString(`<table class="pricing-table">`)
	sb.WriteString(`<tr><th>Feature</th><th>Cost</th></tr>`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News Search</td><td>%d credit%s</td></tr>`, CostNewsSearch, pluralize(CostNewsSearch)))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video Search</td><td>%d credit%s</td></tr>`, CostVideoSearch, pluralize(CostVideoSearch)))
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat AI Query</td><td>%d credit%s</td></tr>`, CostChatQuery, pluralize(CostChatQuery)))
	sb.WriteString(`</table>`)
	sb.WriteString(`<p class="pricing-note"><a href="/plans">View all plans</a> • Members get unlimited access</p>`)
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="transaction-history">`)
		sb.WriteString(`<h3>Transaction History</h3>`)
		sb.WriteString(`<table class="transactions-table">`)
		sb.WriteString(`<tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Type == TxTopup {
				typeLabel = "Top Up"
			}
			amountClass := "debit"
			amountPrefix := "-"
			if tx.Amount > 0 {
				amountClass = "credit"
				amountPrefix = "+"
			}
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td class="%s">%s%d</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountClass, amountPrefix, abs(tx.Amount), tx.Balance))
		}

		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</div>`)

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

	// Add CSS
	sb.WriteString(`
<style>
.wallet-container { max-width: 600px; margin: 0 auto; }
.wallet-balance { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 15px; text-align: center; margin-bottom: 20px; }
.wallet-balance.unlimited { background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%); }
.balance-label { font-size: 14px; opacity: 0.9; margin-bottom: 5px; }
.balance-amount { font-size: 48px; font-weight: bold; }
.balance-credits { font-size: 14px; opacity: 0.9; }
.balance-note { font-size: 14px; margin-top: 10px; }
.daily-quota { background: #f5f5f5; padding: 20px; border-radius: 10px; margin-bottom: 20px; }
.daily-quota h3 { margin: 0 0 15px 0; }
.quota-bar { background: #ddd; height: 20px; border-radius: 10px; overflow: hidden; }
.quota-used { background: linear-gradient(90deg, #667eea, #764ba2); height: 100%; transition: width 0.3s; }
.quota-text { margin-top: 10px; font-weight: bold; }
.quota-note { font-size: 12px; color: #666; margin-top: 5px; }
.topup-section { margin-bottom: 20px; }
.topup-section h3 { margin-bottom: 10px; }
.topup-info { color: #666; font-size: 14px; margin-bottom: 15px; }
.topup-options { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 10px; }
.topup-btn { background: white; border: 2px solid #667eea; border-radius: 10px; padding: 15px 10px; cursor: pointer; transition: all 0.2s; display: flex; flex-direction: column; align-items: center; }
.topup-btn:hover { background: #667eea; color: white; }
.topup-btn:hover .bonus { background: white; color: #667eea; }
.topup-btn .price { font-size: 24px; font-weight: bold; }
.topup-btn .credits { font-size: 12px; color: #666; }
.topup-btn:hover .credits { color: rgba(255,255,255,0.8); }
.topup-btn .bonus { background: #4CAF50; color: white; font-size: 10px; padding: 2px 6px; border-radius: 10px; margin-top: 5px; }
.pricing-info { background: #f9f9f9; padding: 20px; border-radius: 10px; margin-bottom: 20px; }
.pricing-info h3 { margin: 0 0 15px 0; }
.pricing-table { width: 100%; border-collapse: collapse; }
.pricing-table th, .pricing-table td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
.pricing-note { font-size: 12px; color: #666; margin-top: 15px; margin-bottom: 0; }
.transaction-history { margin-top: 20px; }
.transaction-history h3 { margin-bottom: 15px; }
.transactions-table { width: 100%; border-collapse: collapse; font-size: 14px; }
.transactions-table th, .transactions-table td { padding: 10px; text-align: left; border-bottom: 1px solid #eee; }
.transactions-table .credit { color: #4CAF50; }
.transactions-table .debit { color: #f44336; }
</style>`)

	return sb.String()
}

// QuotaExceededPage renders the quota exceeded message
func QuotaExceededPage(operation string, cost int) string {
	var sb strings.Builder

	sb.WriteString(`<div class="quota-exceeded">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your 10 free searches for today.</p>`)
	sb.WriteString(`<div class="options">`)
	sb.WriteString(`<h3>Options</h3>`)
	sb.WriteString(`<ul>`)
	sb.WriteString(`<li>Wait until midnight UTC for more free searches</li>`)
	sb.WriteString(fmt.Sprintf(`<li><a href="/wallet">Use credits</a> (%d credit%s for this search)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li><a href="/plans">View all plans</a></li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`
<style>
.quota-exceeded { max-width: 500px; margin: 50px auto; padding: 30px; background: #f5f5f5; border-radius: 12px; text-align: center; }
.quota-exceeded h2 { color: #333; margin-bottom: 15px; }
.quota-exceeded p { color: #666; }
.quota-exceeded .options { text-align: left; margin-top: 20px; }
.quota-exceeded h3 { font-size: 16px; margin-bottom: 10px; }
.quota-exceeded ul { margin-top: 10px; list-style: none; padding: 0; }
.quota-exceeded li { margin: 12px 0; padding: 10px; background: white; border-radius: 8px; }
.quota-exceeded a { color: #667eea; text-decoration: none; }
.quota-exceeded a:hover { text-decoration: underline; }
</style>`)

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

	checkoutSession, err := session.New(params)
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

	// Show success page
	content := `
<div class="payment-result success">
	<h2>✓ Payment Successful</h2>
	<p>Your credits have been added to your wallet.</p>
	<a href="/wallet" class="btn">View Wallet</a>
</div>
<style>
.payment-result { max-width: 400px; margin: 50px auto; padding: 40px; text-align: center; border-radius: 15px; }
.payment-result.success { background: #d4edda; color: #155724; }
.payment-result h2 { margin-bottom: 15px; }
.payment-result .btn { display: inline-block; margin-top: 20px; padding: 12px 30px; background: #155724; color: white; text-decoration: none; border-radius: 8px; }
</style>`

	html := app.RenderHTMLForRequest("Payment Successful", "Your credits have been added", content, r)
	w.Write([]byte(html))
	_ = sess // Used for potential future user-specific success messages
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	content := `
<div class="payment-result cancelled">
	<h2>Payment Cancelled</h2>
	<p>Your payment was cancelled. No charges were made.</p>
	<a href="/wallet" class="btn">Back to Wallet</a>
</div>
<style>
.payment-result { max-width: 400px; margin: 50px auto; padding: 40px; text-align: center; border-radius: 15px; }
.payment-result.cancelled { background: #fff3cd; color: #856404; }
.payment-result h2 { margin-bottom: 15px; }
.payment-result .btn { display: inline-block; margin-top: 20px; padding: 12px 30px; background: #856404; color: white; text-decoration: none; border-radius: 8px; }
</style>`

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
