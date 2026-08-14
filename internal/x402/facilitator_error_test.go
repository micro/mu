package x402

import (
	"strings"
	"testing"
)

// The message a developer meets on their first x402 call is almost always this
// one, because the commonest first attempt is made from a wallet with nothing
// in it. It has to say so in words that name the fix.
func TestFacilitatorErrorExplainsAnEmptyWallet(t *testing.T) {
	err := &facilitatorError{
		Path:   "/verify",
		Status: 400,
		Body: []byte(`{"invalidMessage":"contract call failed: unable to call contract: execution reverted",` +
			`"invalidReason":"invalid_payload","isValid":false,"payer":"0xD6Aeea79f66C61Bd2Fd1188435c58b2629a0fD9B"}`),
	}

	got := err.Error()
	if !strings.Contains(got, "USDC") {
		t.Errorf("message does not name the missing asset: %s", got)
	}
	if !strings.Contains(got, "0xD6Aeea79f66C61Bd2Fd1188435c58b2629a0fD9B") {
		t.Errorf("message does not name the wallet that must be funded: %s", got)
	}
	// The raw facilitator vocabulary is what made the old message useless.
	if strings.Contains(got, "execution reverted") || strings.Contains(got, "invalid_payload") {
		t.Errorf("message still leaks facilitator internals: %s", got)
	}
}

// Anything else the facilitator says is passed through rather than swallowed —
// guessing at a cause we do not recognise would be worse than repeating it.
func TestFacilitatorErrorPassesThroughOtherReasons(t *testing.T) {
	err := &facilitatorError{
		Path:   "/settle",
		Status: 400,
		Body:   []byte(`{"invalidMessage":"unsupported network","invalidReason":"unsupported_scheme"}`),
	}
	if got := err.Error(); got != "unsupported network" {
		t.Errorf("got %q, want the facilitator's own message", got)
	}
}

// A body that is not JSON at all still has to produce something, and the only
// honest thing left to say is what was sent and what came back.
func TestFacilitatorErrorFallsBackToTheRawBody(t *testing.T) {
	err := &facilitatorError{Path: "/verify", Status: 502, Body: []byte("<html>bad gateway</html>")}
	got := err.Error()
	if !strings.Contains(got, "502") || !strings.Contains(got, "bad gateway") {
		t.Errorf("fallback lost the response: %s", got)
	}
}
