package wallet

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mu/app"
)

var (
	stripeSecretKey     = os.Getenv("STRIPE_SECRET_KEY")
	stripePublicKey     = os.Getenv("STRIPE_PUBLISHABLE_KEY")
	stripeWebhookSecret = os.Getenv("STRIPE_WEBHOOK_SECRET")

	// Track processed sessions to prevent duplicates
	processedSessions = make(map[string]bool)
)

// StripeEnabled returns true if Stripe is configured
func StripeEnabled() bool {
	return stripeSecretKey != "" && stripePublicKey != ""
}

// StripeTopupTier represents a Stripe topup option
type StripeTopupTier struct {
	Amount   int    `json:"amount"`    // Price in pence (e.g., 500 = £5)
	Credits  int    `json:"credits"`   // Credits received
	Label    string `json:"label"`     // Display label
	BonusPct int    `json:"bonus_pct"` // Bonus percentage
}

// StripeTopupTiers - available topup amounts for Stripe
var StripeTopupTiers = []StripeTopupTier{
	{Amount: 500, Credits: 500, Label: "£5", BonusPct: 0},
	{Amount: 1000, Credits: 1050, Label: "£10", BonusPct: 5},
	{Amount: 2500, Credits: 2750, Label: "£25", BonusPct: 10},
	{Amount: 5000, Credits: 5750, Label: "£50", BonusPct: 15},
}

// CreateCheckoutSession creates a Stripe Checkout Session for topup
func CreateCheckoutSession(userID string, amount int, successURL, cancelURL string) (string, error) {
	if !StripeEnabled() {
		return "", fmt.Errorf("stripe not configured")
	}

	// Find tier
	var tier *StripeTopupTier
	for i := range StripeTopupTiers {
		if StripeTopupTiers[i].Amount == amount {
			tier = &StripeTopupTiers[i]
			break
		}
	}
	if tier == nil {
		return "", fmt.Errorf("invalid amount")
	}

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
						"name":        fmt.Sprintf("%d Credits", tier.Credits),
						"description": fmt.Sprintf("Mu credits top-up (%s)", tier.Label),
					},
				},
				"quantity": 1,
			},
		},
		"metadata": map[string]string{
			"user_id": userID,
			"credits": fmt.Sprintf("%d", tier.Credits),
		},
	}

	// Convert to form-urlencoded (Stripe API requires this)
	formData := jsonToForm(data)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(stripeSecretKey, "")
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

	app.Log("stripe", "created checkout session %s for user %s, %d credits", result.ID, userID, tier.Credits)
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

	// Verify webhook signature if secret is configured
	if stripeWebhookSecret != "" {
		sig := r.Header.Get("Stripe-Signature")
		if !verifyStripeSignature(body, sig, stripeWebhookSecret) {
			app.Log("stripe", "webhook signature verification failed")
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}
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
