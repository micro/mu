package x402

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// A challenge says why this particular caller was refused.
//
// The generic x402 wording is surface-neutral because this layer does not know
// whether the public host is a normal Mu instance or X402_HOST. The layer that
// does know the caller and surface can supply a context-specific reason, which
// must pass through unchanged.
func TestAChallengeSaysWhyThisCallerWasRefused(t *testing.T) {
	t.Setenv("X402_ENABLED", "true")
	t.Setenv("X402_PAY_TO", "0x000000000000000000000000000000000000dEaD")

	body := func(reason string) map[string]any {
		w := httptest.NewRecorder()
		if !WritePaymentRequired(w, "web_search", "https://micro.mu/mcp", nil, reason) {
			t.Skip("nothing priced in this configuration")
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("the challenge is not JSON: %v", err)
		}
		return out
	}

	// No reason: use the generic surface-neutral payment guidance.
	anon, _ := body("")["error"].(string)
	if !strings.Contains(anon, "Payment required") ||
		!strings.Contains(anon, "accepts") {
		t.Errorf("the default challenge no longer explains how to pay: %q", anon)
	}

	// A reason: used as given, and the accepts block still rides along, because
	// paying is the way forward that the words are pointing at.
	const mine = "This costs 2 credits and your balance is 0. Top up at https://micro.mu/wallet/topup"
	out := body(mine)
	if got, _ := out["error"].(string); got != mine {
		t.Errorf("error = %q, want %q", got, mine)
	}
	if _, ok := out["accepts"]; !ok {
		t.Error("a refusal with its own wording lost the price and the address to pay")
	}
}
