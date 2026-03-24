package wallet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// x402 protocol headers
const (
	HeaderPaymentRequired = "X-PAYMENT-REQUIRED"
	HeaderPayment         = "X-PAYMENT"
	HeaderPaymentResponse = "X-PAYMENT-RESPONSE"
)

// X402ContextKey is used to mark requests that carry x402 payment
type x402ContextKeyType struct{}

var X402ContextKey = x402ContextKeyType{}

// x402SettleKeyType stores the settlement response in context
type x402SettleKeyType struct{}

var x402SettleKey = x402SettleKeyType{}

// x402 configuration from environment
var (
	x402PayTo          = os.Getenv("X402_PAY_TO")          // Wallet address to receive payments
	x402FacilitatorURL = os.Getenv("X402_FACILITATOR_URL") // Facilitator endpoint
	x402Network        = os.Getenv("X402_NETWORK")         // Blockchain network (e.g., "eip155:8453")
	x402Asset          = os.Getenv("X402_ASSET")           // Token contract address (USDC)
)

func init() {
	if x402FacilitatorURL == "" {
		x402FacilitatorURL = "https://x402.org/facilitator"
	}
	if x402Network == "" {
		x402Network = "eip155:8453" // Base mainnet
	}
	if x402Asset == "" {
		x402Asset = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913" // USDC on Base
	}
}

// X402Enabled returns true if x402 payments are configured
func X402Enabled() bool {
	return x402PayTo != ""
}

// PaymentRequirement describes what payment is needed for a resource
type PaymentRequirement struct {
	Scheme      string `json:"scheme"`
	Network     string `json:"network"`
	MaxAmountRequired string `json:"maxAmountRequired"`
	Resource    string `json:"resource"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
	PayTo       string `json:"payTo"`
	Asset       string `json:"asset"`
}

// VerifyResponse is the facilitator's verification response
type VerifyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// SettleResponse is the facilitator's settlement response
type SettleResponse struct {
	Success     bool   `json:"success"`
	Transaction string `json:"transaction,omitempty"`
	Network     string `json:"network,omitempty"`
	Message     string `json:"message,omitempty"`
}

// CreditsToDollars converts credit cost (pennies GBP) to USD string
// 1 credit = 1p GBP ≈ $0.013 USD (approximate, configurable)
func CreditsToDollars(credits int) string {
	// Use a simple rate: 1 credit = $0.01 USD (close enough, keeps it clean)
	cents := credits
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

// BuildPaymentRequirements creates the 402 response payload for an operation
func BuildPaymentRequirements(operation string, resource string) []PaymentRequirement {
	cost := GetOperationCost(operation)
	return []PaymentRequirement{{
		Scheme:            "exact",
		Network:           x402Network,
		MaxAmountRequired: CreditsToDollars(cost),
		Resource:          resource,
		Description:       fmt.Sprintf("Access to %s", operation),
		MimeType:          "application/json",
		PayTo:             x402PayTo,
		Asset:             x402Asset,
	}}
}

// WritePaymentRequired sends a 402 response with x402 payment requirements
func WritePaymentRequired(w http.ResponseWriter, operation string, resource string) {
	requirements := BuildPaymentRequirements(operation, resource)
	b, _ := json.Marshal(requirements)
	encoded := base64.StdEncoding.EncodeToString(b)

	w.Header().Set(HeaderPaymentRequired, encoded)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   "Payment required",
		"x402":    requirements,
		"accepts": []string{"x402"},
	})
}

// HasPayment checks if a request includes an x402 payment header
func HasPayment(r *http.Request) bool {
	return r.Header.Get(HeaderPayment) != ""
}

// VerifyAndSettle verifies payment via facilitator and settles if valid.
// Returns true if payment was successfully verified and settled.
func VerifyAndSettle(r *http.Request, operation string, resource string) (*SettleResponse, error) {
	paymentHeader := r.Header.Get(HeaderPayment)
	if paymentHeader == "" {
		return nil, fmt.Errorf("no payment header")
	}

	requirements := BuildPaymentRequirements(operation, resource)
	reqBytes, _ := json.Marshal(requirements)

	// Step 1: Verify via facilitator
	verifyResp, err := facilitatorRequest("/verify", map[string]any{
		"payload":      paymentHeader,
		"requirements": base64.StdEncoding.EncodeToString(reqBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	var verify VerifyResponse
	if err := json.Unmarshal(verifyResp, &verify); err != nil {
		return nil, fmt.Errorf("invalid verify response: %w", err)
	}
	if !verify.Valid {
		return nil, fmt.Errorf("payment invalid: %s", verify.Message)
	}

	// Step 2: Settle via facilitator
	settleResp, err := facilitatorRequest("/settle", map[string]any{
		"payload":      paymentHeader,
		"requirements": base64.StdEncoding.EncodeToString(reqBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("settlement failed: %w", err)
	}

	var settle SettleResponse
	if err := json.Unmarshal(settleResp, &settle); err != nil {
		return nil, fmt.Errorf("invalid settle response: %w", err)
	}
	if !settle.Success {
		return nil, fmt.Errorf("settlement failed: %s", settle.Message)
	}

	log.Printf("[x402] payment settled for %s: tx=%s", operation, settle.Transaction)
	return &settle, nil
}

// facilitatorRequest makes a POST request to the x402 facilitator
func facilitatorRequest(path string, body map[string]any) ([]byte, error) {
	b, _ := json.Marshal(body)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(x402FacilitatorURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilitator returned %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}
