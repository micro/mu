package wallet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWritePaymentRequiredHTTP verifies the server side of the x402 handshake:
// a metered call with no payment gets a well-formed HTTP 402 whose JSON body is
// the payment challenge every x402 client expects — status, content type, the
// x402Version/error/accepts envelope, and a price that reflects the operation's
// real credit cost. This is the deterministic half of "does x402 work"; the
// paying half (sign → retry → settle) is covered by TestPayAndCallMCP.
func TestWritePaymentRequiredHTTP(t *testing.T) {
	x402PayTo = "0x9a717EFF039622231C65ADbF7B2A002b544b06A9"
	x402NetworkID = "eip155:8453"
	defer func() { x402PayTo = ""; x402NetworkID = "" }()

	const op = "agent_query"
	const resource = "https://micro.mu/agent/run"

	rec := httptest.NewRecorder()
	WritePaymentRequired(rec, op, resource)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body struct {
		X402Version int                   `json:"x402Version"`
		Error       string                `json:"error"`
		Accepts     []PaymentRequirements `json:"accepts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body.X402Version != x402Version {
		t.Errorf("x402Version = %d, want %d", body.X402Version, x402Version)
	}
	if body.Error == "" {
		t.Error("error message must be present so clients can surface it")
	}
	if len(body.Accepts) == 0 {
		t.Fatal("accepts must list at least one payment requirement")
	}

	got := body.Accepts[0]
	if got.Scheme != "exact" {
		t.Errorf("scheme = %q, want exact", got.Scheme)
	}
	if got.PayTo != x402PayTo {
		t.Errorf("payTo = %q, want %q", got.PayTo, x402PayTo)
	}
	if got.Network != x402NetworkID {
		t.Errorf("network = %q, want %q", got.Network, x402NetworkID)
	}
	if got.Resource != resource {
		t.Errorf("resource = %q, want %q", got.Resource, resource)
	}
	// The advertised price must equal what the builder derives from the op's
	// real credit cost — the HTTP envelope must not drift from the pricing.
	want := BuildPaymentRequirements(op, resource)
	if len(want) == 0 || got.MaxAmountRequired != want[0].MaxAmountRequired {
		t.Errorf("maxAmountRequired = %q, want %q (from op cost)", got.MaxAmountRequired, want[0].MaxAmountRequired)
	}
	if got.MaxAmountRequired == "" {
		t.Error("price must be set (atomic units)")
	}
}
