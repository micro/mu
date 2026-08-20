package x402

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
	t.Setenv("X402_NETWORK", "eip155:8453")
	defer func() { x402PayTo = "" }()

	// A priced operation, because the challenge is built from the price and
	// there is nothing to charge for a free one. This said agent_query, which
	// stopped being priced when the agent stopped being a metered capability —
	// see quota.json. Any of the third-party operations does; web search is the
	// one the README uses.
	const op = "web_search"
	const resource = "https://micro.mu/mcp"

	rec := httptest.NewRecorder()
	WritePaymentRequired(rec, op, resource, nil, "")

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
		// v2 names the resource once at the top rather than in every
		// requirement. v1 leaves this absent and fills the field below.
		Resource *struct {
			URL         string `json:"url"`
			ServiceName string `json:"serviceName"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body.X402Version != x402Ver() {
		t.Errorf("x402Version = %d, want %d", body.X402Version, x402Ver())
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
	if got.Network != Network() {
		t.Errorf("network = %q, want %q", got.Network, Network())
	}
	// Whichever version is advertised, the challenge has to say what is being
	// bought — a client that cannot tell which URL it is paying for cannot
	// check the price is for the thing it asked for.
	switch {
	case body.X402Version >= 2:
		if body.Resource == nil || body.Resource.URL != resource {
			t.Errorf("v2 resource.url = %v, want %q", body.Resource, resource)
		}
		if body.Resource != nil && body.Resource.ServiceName == "" {
			t.Error("v2 resource.serviceName is empty, so a listing has no name")
		}
		if got.Resource != "" {
			t.Errorf("v2 requirement still carries the v1 resource field: %q", got.Resource)
		}
	default:
		if got.Resource != resource {
			t.Errorf("resource = %q, want %q", got.Resource, resource)
		}
	}
	// The advertised price must equal what the builder derives from the op's
	// real credit cost — the HTTP envelope must not drift from the pricing.
	want := BuildPaymentRequirements(op, resource)
	if len(want) == 0 || got.AmountAtomic() != want[0].AmountAtomic() {
		t.Errorf("amount = %q, want %q (from op cost)", got.AmountAtomic(), want[0].AmountAtomic())
	}
	if got.AmountAtomic() == "" {
		t.Error("price must be set (atomic units)")
	}
}
