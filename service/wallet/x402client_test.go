package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/x402"
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
			"accepts": []x402.PaymentRequirements{{
				Scheme: "exact", Network: "base", MaxAmountRequired: "10000",
				PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
				MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
			}},
		})
	}))
	defer srv.Close()

	out, err := PayAndCallMCP(context.Background(), "acct-test", srv.URL, "news_search", map[string]any{"query": "btc"}, bw)
	if err != nil {
		t.Fatalf("PayAndCallMCP: %v", err)
	}
	if out != "bitcoin is up" {
		t.Errorf("result = %q", out)
	}
	raw, err := base64.StdEncoding.DecodeString(sawPayment)
	if err != nil {
		t.Fatalf("payment not base64: %v", err)
	}
	if !strings.Contains(string(raw), `"scheme":"exact"`) || !strings.Contains(strings.ToLower(string(raw)), strings.ToLower(addr[2:])) {
		t.Errorf("payment payload missing scheme/from: %s", raw)
	}
}

func TestPayAndCallMCPWithReceipt(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	settle := x402.SettleResponse{
		Success: true, Transaction: "0xabc123", Network: "eip155:8453", Payer: addr,
	}
	settleJSON, _ := json.Marshal(settle)
	settleHeader := base64.StdEncoding.EncodeToString(settleJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-PAYMENT") != "" {
			w.Header().Set(x402.HeaderPaymentRespV2, settleHeader)
			w.Header().Set("EXTENSION-RESPONSES", `{"bazaar":{"status":"processing"}}`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "paid result"}}},
			})
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"x402Version": 2, "error": "payment required",
			"accepts": []x402.PaymentRequirements{{
				Scheme: "exact", Network: "eip155:8453", Amount: "10000",
				PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
				MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
			}},
		})
	}))
	defer srv.Close()

	out, err := PayAndCallMCPWithReceipt(context.Background(), "acct-test", srv.URL, "web_search", map[string]any{"query": "x402"}, bw)
	if err != nil {
		t.Fatalf("PayAndCallMCPWithReceipt: %v", err)
	}
	if out.Text != "paid result" {
		t.Fatalf("text = %q", out.Text)
	}
	if out.Settlement == nil || out.Settlement.Transaction != "0xabc123" || out.Settlement.Network != "eip155:8453" {
		t.Fatalf("settlement = %#v", out.Settlement)
	}
	if out.PaymentError != "" {
		t.Fatalf("payment error = %q", out.PaymentError)
	}
	if out.ExtensionResponses != `{"bazaar":{"status":"processing"}}` {
		t.Fatalf("extension responses = %q", out.ExtensionResponses)
	}
}

func TestPayAndCallMCPWithFailedReceiptStillReportsExtensions(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	settle := x402.SettleResponse{Success: false, ErrorReason: "insufficient funds"}
	settleJSON, _ := json.Marshal(settle)
	settleHeader := base64.StdEncoding.EncodeToString(settleJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-PAYMENT") != "" {
			w.Header().Set(x402.HeaderPaymentRespV2, settleHeader)
			w.Header().Set("EXTENSION-RESPONSES", `{"bazaar":{"status":"processing"}}`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "tool result"}}},
			})
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"x402Version": 2, "error": "payment required",
			"accepts": []x402.PaymentRequirements{{
				Scheme: "exact", Network: "eip155:8453", Amount: "10000",
				PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
				MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
			}},
		})
	}))
	defer srv.Close()

	out, err := PayAndCallMCPWithReceipt(context.Background(), "acct-test", srv.URL, "web_search", map[string]any{"query": "x402"}, bw)
	if err != nil {
		t.Fatalf("PayAndCallMCPWithReceipt: %v", err)
	}
	if out.Settlement != nil {
		t.Fatalf("failed settlement reported as success: %#v", out.Settlement)
	}
	if out.PaymentError != "insufficient funds" {
		t.Fatalf("payment error = %q", out.PaymentError)
	}
	if out.ExtensionResponses != `{"bazaar":{"status":"processing"}}` {
		t.Fatalf("extension responses = %q", out.ExtensionResponses)
	}
}

func TestSettlementFromHeadersRejectsIncompleteSuccess(t *testing.T) {
	settleJSON, _ := json.Marshal(x402.SettleResponse{Success: true})
	h := http.Header{}
	h.Set(x402.HeaderPaymentRespV2, base64.StdEncoding.EncodeToString(settleJSON))

	settle, paymentErr := settlementFromHeaders(h)
	if settle != nil {
		t.Fatalf("incomplete receipt reported as success: %#v", settle)
	}
	if paymentErr != "incomplete PAYMENT-RESPONSE: missing transaction or network" {
		t.Fatalf("payment error = %q", paymentErr)
	}
}

func TestPayAndCallMCPNoWallet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"accepts":[]}`))
	}))
	defer srv.Close()
	if _, err := PayAndCallMCP(context.Background(), "acct-test", srv.URL, "x", nil, nil); err == nil {
		t.Error("expected error when payment required but no wallet")
	}
}
