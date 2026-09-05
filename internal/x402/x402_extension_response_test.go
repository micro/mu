package x402

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettleWriterForwardsExtensionResponses(t *testing.T) {
	oldFacilitator := x402FacilitatorURL
	defer func() { x402FacilitatorURL = oldFacilitator }()

	facilitator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/settle" {
			t.Fatalf("unexpected facilitator path: %s", r.URL.Path)
		}
		w.Header().Set(HeaderExtensionResponses, `{"bazaar":{"status":"processing"}}`)
		_ = json.NewEncoder(w).Encode(SettleResponse{
			Success:     true,
			Transaction: "0xabc123",
			Network:     "eip155:8453",
			Payer:       "0x1234",
		})
	}))
	defer facilitator.Close()
	x402FacilitatorURL = facilitator.URL

	holder := &SettleHolder{Verified: &Verified{
		payload: map[string]any{},
		req:     &PaymentRequirements{Resource: "https://m3o.com/mcp"},
	}}
	rec := httptest.NewRecorder()
	w := NewSettleWriter(rec, holder)

	_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
	Finish(w)

	if got := rec.Header().Get(HeaderExtensionResponses); got != `{"bazaar":{"status":"processing"}}` {
		t.Fatalf("%s = %q", HeaderExtensionResponses, got)
	}
	if rec.Header().Get(HeaderPaymentRespV2) == "" {
		t.Fatal("missing payment response header")
	}
}
