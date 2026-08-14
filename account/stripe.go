package account

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"mu/internal/quota"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

var (
	processedSessions = make(map[string]bool)
)

func stripeSecret() string  { return settings.Get("STRIPE_SECRET_KEY") }
func stripePublic() string  { return settings.Get("STRIPE_PUBLISHABLE_KEY") }
func stripeWebhook() string { return settings.Get("STRIPE_WEBHOOK_SECRET") }

func StripeEnabled() bool {
	return stripeSecret() != "" && stripePublic() != ""
}

// StripeTopupTier is one preset top-up amount.
type StripeTopupTier struct {
	Amount  int    `json:"amount"`  // Price in pence (e.g., 500 = £5)
	Credits int    `json:"credits"` // Credits received (equals Amount, flat rate)
	Label   string `json:"label"`   // Display label
}

// StripeTopupTiers are the presets on the top-up form.
//
// Not tiers in the pricing sense — a credit is a penny at every one of them,
// and any amount can be typed instead. They are there because most people would
// rather press £10 than think of a number.
var StripeTopupTiers = []StripeTopupTier{
	{Amount: 500, Credits: 500, Label: "£5"},
	{Amount: 1000, Credits: 1000, Label: "£10"},
	{Amount: 2500, Credits: 2500, Label: "£25"},
	{Amount: 5000, Credits: 5000, Label: "£50"},
}

// checkoutSession is a completed purchase, however we came to hear about it.
type checkoutSession struct {
	ID            string
	PaymentStatus string
	Customer      string
	AmountTotal   int
	UserID        string
	Credits       string
}

// settleSession applies a paid checkout: credits, and who Stripe thinks the
// customer is.
//
// Extracted because the webhook was the only thing that could do it, and a
// webhook is a promise from a service you do not control. If it is not
// configured, or its secret is wrong, or its event list is missing the one that
// matters, the customer's card is charged and nothing whatever happens here —
// no credits, no plan, no error, and no way to find out but asking them why
// their balance is zero. That is not a subscription problem: a one-off top-up
// hangs on exactly the same thread.
//
// So the return from Stripe settles it too, and the two are safe to race
// because this is the only place it happens and it runs once per session id.
// The dedupe is what makes a second route free rather than dangerous.
//
// how is only for the log — knowing whether the money landed by webhook or by
// somebody coming back to the page is the difference between "Stripe is fine"
// and "Stripe has never once called us".
func settleSession(s checkoutSession, how string) {
	if s.PaymentStatus != "paid" || s.ID == "" {
		return
	}

	mutex.Lock()
	if processedSessions[s.ID] {
		mutex.Unlock()
		return
	}
	processedSessions[s.ID] = true
	mutex.Unlock()

	if s.UserID == "" {
		app.Log("stripe", "session %s (%s) names no account, so nothing can be credited", s.ID, how)
		return
	}

	var credits int
	fmt.Sscanf(s.Credits, "%d", &credits)
	if credits > 0 {
		if err := AddCredits(s.UserID, credits, quota.OpTopup, map[string]interface{}{
			"source":     "stripe",
			"session_id": s.ID,
			"amount":     s.AmountTotal,
			"settled_by": how,
		}); err != nil {
			app.Log("stripe", "failed to credit user %s: %v", s.UserID, err)
		} else {
			app.Log("stripe", "credited %d to %s via %s (session %s)", credits, s.UserID, how, s.ID)
		}
	}
	// Kept because a top-up creates a Stripe customer, and having the id means a
	// card saved on one purchase is recognisable on the next.
	setCustomer(s.UserID, s.Customer)
}

// SettleCheckout reads a session back from Stripe and applies it.
//
// What the return from a payment calls. The session id arrives in the URL
// Stripe redirects to, and everything acted on comes from asking Stripe about
// it rather than from the query string — a caller can type any id they like
// into a URL, and the answer is only trustworthy because Stripe gave it.
func SettleCheckout(sessionID, forAccount string) error {
	if !StripeEnabled() || strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("no session")
	}
	req, err := http.NewRequest("GET",
		"https://api.stripe.com/v1/checkout/sessions/"+neturl.PathEscape(sessionID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out struct {
		ID            string `json:"id"`
		PaymentStatus string `json:"payment_status"`
		Customer      string `json:"customer"`
		AmountTotal   int    `json:"amount_total"`
		Metadata      struct {
			UserID  string `json:"user_id"`
			Credits string `json:"credits"`
		} `json:"metadata"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Error.Message != "" {
		return fmt.Errorf("%s", out.Error.Message)
	}
	// The session has to belong to whoever is asking. Session ids are not
	// secrets — one arrives in a URL and sits in a browser history — so without
	// this, pasting somebody else's would apply their purchase to your account.
	if forAccount != "" && out.Metadata.UserID != forAccount {
		return fmt.Errorf("that payment belongs to another account")
	}

	settleSession(checkoutSession{
		ID:            out.ID,
		PaymentStatus: out.PaymentStatus,
		Customer:      out.Customer,
		AmountTotal:   out.AmountTotal,
		UserID:        out.Metadata.UserID,
		Credits:       out.Metadata.Credits,
	}, "return from checkout")
	return nil
}

// setCustomer records who Stripe thinks an account is.
//
// Written once and then left alone. Stripe reuses a customer across purchases,
// so the first one is the one that holds the cards and the subscriptions;
// overwriting it on a later checkout that happened to mint a second would
// point the billing portal at the emptier of the two.
func setCustomer(userID, customerID string) {
	if userID == "" || !strings.HasPrefix(customerID, "cus_") {
		return
	}
	acc, err := auth.GetAccount(userID)
	if err != nil || acc.Customer == customerID {
		return
	}
	if acc.Customer != "" {
		return
	}
	acc.Customer = customerID
	if err := auth.UpdateAccount(acc); err != nil {
		app.Log("stripe", "failed to record customer %s for %s: %v", customerID, userID, err)
		return
	}
	app.Log("stripe", "%s is stripe customer %s", userID, customerID)
}

// CreateCheckoutSession creates a Stripe Checkout Session for topup
func CreateCheckoutSession(userID string, amount int, successURL, cancelURL string) (string, error) {
	if !StripeEnabled() {
		return "", fmt.Errorf("stripe not configured")
	}

	if amount < 100 {
		return "", fmt.Errorf("minimum top-up is £1")
	}

	// Flat rate: 1 pence = 1 credit
	credits := amount
	label := fmt.Sprintf("£%d", amount/100)

	// Build request body
	data := map[string]interface{}{
		"mode":        "payment",
		"success_url": successURL,
		"cancel_url":  cancelURL,
		"line_items": []map[string]interface{}{
			{
				"price_data": map[string]interface{}{
					"currency":    "gbp",
					"unit_amount": amount,
					"product_data": map[string]interface{}{
						"name":        fmt.Sprintf("%d Credits", credits),
						"description": fmt.Sprintf("Mu credits top-up (%s)", label),
					},
				},
				"quantity": 1,
			},
		},
		"metadata": map[string]string{
			"user_id": userID,
			"credits": fmt.Sprintf("%d", credits),
		},
	}

	// Convert to form-urlencoded (Stripe API requires this)
	formData := jsonToForm(data)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(stripeSecret(), "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		app.Log("stripe", "checkout session error: %s", string(body))
		return "", fmt.Errorf("stripe error: %s", resp.Status)
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	app.Log("stripe", "created checkout session %s for user %s, %d credits", result.ID, userID, credits)
	return result.URL, nil
}

// jsonToForm converts nested JSON to Stripe's form-urlencoded format
func jsonToForm(data map[string]interface{}) string {
	var parts []string
	encodeValue("", data, &parts)
	return strings.Join(parts, "&")
}

func encodeValue(prefix string, v interface{}, parts *[]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, v := range val {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "[" + k + "]"
			}
			encodeValue(newPrefix, v, parts)
		}
	case map[string]string:
		for k, v := range val {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "[" + k + "]"
			}
			*parts = append(*parts, fmt.Sprintf("%s=%s", newPrefix, urlEncode(v)))
		}
	case []map[string]interface{}:
		for i, item := range val {
			newPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			encodeValue(newPrefix, item, parts)
		}
	case string:
		*parts = append(*parts, fmt.Sprintf("%s=%s", prefix, urlEncode(val)))
	case int:
		*parts = append(*parts, fmt.Sprintf("%s=%d", prefix, val))
	case int64:
		*parts = append(*parts, fmt.Sprintf("%s=%d", prefix, val))
	case float64:
		*parts = append(*parts, fmt.Sprintf("%s=%v", prefix, val))
	}
}

func urlEncode(s string) string {
	// Simple URL encoding for common characters
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, "&", "%26")
	s = strings.ReplaceAll(s, "=", "%3D")
	s = strings.ReplaceAll(s, "+", "%2B")
	return s
}

// HandleStripeWebhook processes Stripe webhook events
func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify webhook signature — REQUIRED for security
	if stripeWebhook() == "" {
		app.Log("stripe", "CRITICAL: STRIPE_WEBHOOK_SECRET not configured, rejecting webhook")
		http.Error(w, "webhook not configured", http.StatusServiceUnavailable)
		return
	}
	sig := r.Header.Get("Stripe-Signature")
	if !verifyStripeSignature(body, sig, stripeWebhook()) {
		app.Log("stripe", "webhook signature verification failed")
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	// Parse event
	var event struct {
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		app.Log("stripe", "webhook parse error: %v", err)
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	app.Log("stripe", "webhook received: %s", event.Type)

	// Handle checkout.session.completed
	if event.Type == "checkout.session.completed" {
		var session struct {
			ID            string `json:"id"`
			PaymentStatus string `json:"payment_status"`
			// Customer is who Stripe thinks this is, and it is the only handle
			// on a subscription after it exists. Without it there is no way to
			// open a billing portal, so there is no way to cancel — which is
			// what this account had: a monthly charge with no customer-side
			// exit but a failed card or a chargeback.
			Customer string `json:"customer"`
			Metadata struct {
				UserID  string `json:"user_id"`
				Credits string `json:"credits"`
			} `json:"metadata"`
			AmountTotal int `json:"amount_total"`
		}
		if err := json.Unmarshal(event.Data.Object, &session); err != nil {
			app.Log("stripe", "session parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		settleSession(checkoutSession{
			ID:            session.ID,
			PaymentStatus: session.PaymentStatus,
			Customer:      session.Customer,
			AmountTotal:   session.AmountTotal,
			UserID:        session.Metadata.UserID,
			Credits:       session.Metadata.Credits,
		}, "webhook")
	}

	w.WriteHeader(http.StatusOK)
}

// verifyStripeSignature verifies the Stripe webhook signature
func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	if sigHeader == "" {
		return false
	}

	// Parse signature header
	var timestamp string
	var signatures []string

	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	// Check timestamp is within tolerance (5 minutes)
	var ts int64
	fmt.Sscanf(timestamp, "%d", &ts)
	now := time.Now().Unix()
	if now-ts > 300 || ts-now > 300 {
		app.Log("stripe", "webhook timestamp out of tolerance: %d vs %d", ts, now)
		return false
	}

	// Verify at least one signature matches
	signedPayload := timestamp + "." + string(payload)
	expectedSig := computeHMAC(signedPayload, secret)

	for _, sig := range signatures {
		if secureCompare(sig, expectedSig) {
			return true
		}
	}

	return false
}

func computeHMAC(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
