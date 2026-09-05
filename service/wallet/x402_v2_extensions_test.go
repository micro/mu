package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mu/internal/x402"
)

func TestSignX402PaymentWithContextEchoesV2ResourceAndExtensions(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	req := x402.PaymentRequirements{
		Scheme: "exact", Network: "eip155:8453", Amount: "30000",
		PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
		MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
	}
	resource := map[string]any{"url": "https://m3o.com/mcp", "description": "Access to web_search", "mimeType": "application/json"}
	extensions := map[string]any{"bazaar": map[string]any{"info": map[string]any{"input": map[string]any{"type": "mcp", "toolName": "web_search"}}}}

	hdr, err := SignX402PaymentWithContext(bw, req, resource, extensions)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(hdr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	gotResource, _ := payload["resource"].(map[string]any)
	if gotResource["url"] != "https://m3o.com/mcp" {
		t.Fatalf("resource = %#v", gotResource)
	}
	gotExtensions, _ := payload["extensions"].(map[string]any)
	if _, ok := gotExtensions["bazaar"]; !ok {
		t.Fatalf("extensions = %#v", gotExtensions)
	}
}

func TestPayAndCallMCPEchoesChallengeExtensions(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	var paidPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pay := r.Header.Get("X-PAYMENT"); pay != "" {
			raw, _ := base64.StdEncoding.DecodeString(pay)
			_ = json.Unmarshal(raw, &paidPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}},
			})
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"x402Version": 2,
			"accepts": []x402.PaymentRequirements{{
				Scheme: "exact", Network: "eip155:8453", Amount: "30000",
				PayTo: "0x9a717EFF039622231C65ADbF7B2A002b544b06A9", Asset: baseUSDC,
				MaxTimeoutSeconds: 60, Extra: map[string]string{"name": "USD Coin", "version": "2"},
			}},
			"resource": map[string]any{"url": "https://m3o.com/mcp", "description": "Access to web_search", "mimeType": "application/json"},
			"extensions": map[string]any{"bazaar": map[string]any{"info": map[string]any{"input": map[string]any{"type": "mcp", "toolName": "web_search"}}}},
		})
	}))
	defer srv.Close()

	out, err := PayAndCallMCP(context.Background(), "acct-v2-ext", srv.URL, "web_search", map[string]any{"query": "x402"}, bw)
	if err != nil {
		t.Fatalf("PayAndCallMCP: %v", err)
	}
	if out != "ok" {
		t.Fatalf("result = %q", out)
	}
	if paidPayload == nil {
		t.Fatal("paid retry did not carry a decodable payment payload")
	}
	if _, ok := paidPayload["resource"]; !ok {
		t.Fatalf("payment payload missing resource: %#v", paidPayload)
	}
	ext, _ := paidPayload["extensions"].(map[string]any)
	if _, ok := ext["bazaar"]; !ok {
		t.Fatalf("payment payload missing bazaar extension: %#v", paidPayload)
	}
}
