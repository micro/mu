package payment

import (
	"mu/internal/usage"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlySuccessfulInboundSettlementRecordsPayment(t *testing.T) {
	success := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if success {
			w.Write([]byte(`{"success":true,"transaction":"audit-tx","payer":"audit-payer"}`))
		} else {
			w.Write([]byte(`{"success":false,"errorReason":"denied"}`))
		}
	}))
	defer server.Close()
	old := x402FacilitatorURL
	x402FacilitatorURL = server.URL
	defer func() { x402FacilitatorURL = old }()
	v := &Verified{host: "audit.example", path: "/mcp", operation: "search", payload: map[string]any{}, req: &PaymentRequirements{Amount: "123", Asset: "asset", Network: "network"}}
	count := func() int {
		n := 0
		for _, a := range usage.Activities(false) {
			if a.Host == "audit.example" && a.Surface == "payment" {
				n++
			}
		}
		return n
	}
	before := count()
	if _, err := v.Settle(); err == nil || count() != before {
		t.Fatal("failed settlement recorded")
	}
	success = true
	if _, err := v.Settle(); err != nil {
		t.Fatal(err)
	}
	if count() != before+1 {
		t.Fatal("successful settlement missing")
	}
	// Outbound settlement uses settleVerified directly and must not look like income.
	if _, err := settleVerified(v.payload, v.req); err != nil {
		t.Fatal(err)
	}
	if count() != before+1 {
		t.Fatal("outbound payment recorded as inbound")
	}
}
