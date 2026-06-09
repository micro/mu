package wallet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/settings"
)

var (
	processedSessions = make(map[string]bool)
)

func stripeSecret() string    { return settings.Get("STRIPE_SECRET_KEY") }
func stripePublic() string    { return settings.Get("STRIPE_PUBLISHABLE_KEY") }
func stripeWebhook() string   { return settings.Get("STRIPE_WEBHOOK_SECRET") }

func StripeEnabled() bool {
	return stripeSecret() != "" && stripePublic() != ""
}

// Subscription plans — monthly credit bundles via Stripe.
type SubscriptionPlan struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`   // Monthly price in pence
	Credits int    `json:"credits"` // Credits granted each month
	Label   string `json:"label"`
}

var SubscriptionPlans = []SubscriptionPlan{
	{ID: "starter", Name: "Starter", Price: 500, Credits: 500, Label: "£5/month — 500 credits"},
	{ID: "pro", Name: "Pro", Price: 1000, Credits: 1200, Label: "£10/month — 1,200 credits"},
}

// CreateSubscriptionSession creates a Stripe Checkout Session for a
// recurring subscription. Credits are granted on each successful payment
// via the invoice.payment_succeeded webhook.
func CreateSubscriptionSession(userID, planID, successURL, cancelURL string) (string, error) {
	if !StripeEnabled() {
		return "", fmt.Errorf("stripe not configured")
	}

	var plan *SubscriptionPlan
	for i := range SubscriptionPlans {
		if SubscriptionPlans[i].ID == planID {
			plan = &SubscriptionPlans[i]
			break
		}
	}
	if plan == nil {
		return "", fmt.Errorf("unknown plan: %s", planID)
	}

	data := map[string]interface{}{
		"mode":        "subscription",
		"success_url": successURL,
		"cancel_url":  cancelURL,
		"line_items": []map[string]interface{}{
			{
				"price_data": map[string]interface{}{
					"currency": "gbp",
					"recurring": map[string]interface{}{
						"interval": "month",
					},
					"unit_amount": plan.Price,
					"product_data": map[string]interface{}{
						"name":        plan.Name + " Plan",
						"description": fmt.Sprintf("%d credits/month", plan.Credits),
					},
				},
				"quantity": 1,
			},
		},
		"metadata": map[string]string{
			"user_id": userID,
			"plan_id": plan.ID,
			"credits": fmt.Sprintf("%d", plan.Credits),
		},
		"subscription_data": map[string]interface{}{
			"metadata": map[string]string{
				"user_id": userID,
				"plan_id": plan.ID,
				"credits": fmt.Sprintf("%d", plan.Credits),
			},
		},
	}

	formData := jsonToForm(data)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+stripeSecret())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error.Message != "" {
		return "", fmt.Errorf("stripe: %s", result.Error.Message)
	}
	return result.URL, nil
}

// StripeTopupTier represents a Stripe topup option
type StripeTopupTier struct {
	Amount  int    `json:"amount"`  // Price in pence (e.g., 500 = £5)
	Credits int    `json:"credits"` // Credits received (equals Amount, flat rate)
	Label   string `json:"label"`   // Display label
}

// StripeTopupTiers - preset topup amounts for Stripe
var StripeTopupTiers = []StripeTopupTier{
	{Amount: 500, Credits: 500, Label: "£5"},
	{Amount: 1000, Credits: 1000, Label: "£10"},
	{Amount: 2500, Credits: 2500, Label: "£25"},
	{Amount: 5000, Credits: 5000, Label: "£50"},
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
			Metadata      struct {
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

		if session.PaymentStatus == "paid" {
			// Check for duplicate processing
			mutex.Lock()
			if processedSessions[session.ID] {
				mutex.Unlock()
				app.Log("stripe", "session %s already processed, skipping", session.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			processedSessions[session.ID] = true
			mutex.Unlock()

			userID := session.Metadata.UserID
			var credits int
			fmt.Sscanf(session.Metadata.Credits, "%d", &credits)

			if userID != "" && credits > 0 {
				err := AddCredits(userID, credits, OpTopup, map[string]interface{}{
					"source":     "stripe",
					"session_id": session.ID,
					"amount":     session.AmountTotal,
				})
				if err != nil {
					app.Log("stripe", "failed to credit user %s: %v", userID, err)
				} else {
					app.Log("stripe", "credited %d to user %s (session %s)", credits, userID, session.ID)
				}
			}
		}
	}

	// Handle invoice.payment_succeeded — subscription renewal credits.
	if event.Type == "invoice.payment_succeeded" {
		var invoice struct {
			ID               string `json:"id"`
			SubscriptionData struct {
				Metadata struct {
					UserID  string `json:"user_id"`
					Credits string `json:"credits"`
					PlanID  string `json:"plan_id"`
				} `json:"metadata"`
			} `json:"subscription_details"`
			Subscription string `json:"subscription"`
		}
		if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
			app.Log("stripe", "invoice parse error: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		userID := invoice.SubscriptionData.Metadata.UserID
		var credits int
		fmt.Sscanf(invoice.SubscriptionData.Metadata.Credits, "%d", &credits)

		if userID != "" && credits > 0 {
			// Dedup by invoice ID.
			mutex.Lock()
			if processedSessions[invoice.ID] {
				mutex.Unlock()
				app.Log("stripe", "invoice %s already processed, skipping", invoice.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			processedSessions[invoice.ID] = true
			mutex.Unlock()

			err := AddCredits(userID, credits, OpTopup, map[string]interface{}{
				"source":       "stripe_subscription",
				"invoice_id":   invoice.ID,
				"subscription": invoice.Subscription,
				"plan":         invoice.SubscriptionData.Metadata.PlanID,
			})
			if err != nil {
				app.Log("stripe", "failed to credit subscriber %s: %v", userID, err)
			} else {
				app.Log("stripe", "subscription: credited %d to %s (invoice %s)", credits, userID, invoice.ID)
			}
		}
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
