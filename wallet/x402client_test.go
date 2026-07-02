package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPayAndCallMCP drives the full payer loop: 402 challenge → sign → retry →
// result, verifying the retry carried a well-formed X-PAYMENT for the offered
// requirement.
func TestPayAndCallMCP(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	var sawPayment string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.Header.Get("X-PAYMENT"); p != "" {
			sawPayment = p
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "bitcoin is up"}}},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"x402Version": 1, "error": "payment required",
			"accepts": []PaymentRequirements{{
				Scheme: "exact", Network: "base", MaxAmountRequired: "10000",
				PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
				MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
			}},
		})
	}))
	defer srv.Close()

	out, err := PayAndCallMCP(context.Background(), srv.URL, "news_search", map[string]any{"query": "btc"}, bw)
	if err != nil {
		t.Fatalf("PayAndCallMCP: %v", err)
	}
	if out != "bitcoin is up" {
		t.Errorf("result = %q", out)
	}
	// The retry payment must decode to an exact-scheme payload paying from us.
	raw, err := base64.StdEncoding.DecodeString(sawPayment)
	if err != nil {
		t.Fatalf("payment not base64: %v", err)
	}
	if !strings.Contains(string(raw), `"scheme":"exact"`) || !strings.Contains(strings.ToLower(string(raw)), strings.ToLower(addr[2:])) {
		t.Errorf("payment payload missing scheme/from: %s", raw)
	}
}

func TestPayAndCallMCPNoWallet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"accepts":[]}`))
	}))
	defer srv.Close()
	if _, err := PayAndCallMCP(context.Background(), srv.URL, "x", nil, nil); err == nil {
		t.Error("expected error when payment required but no wallet")
	}
}
