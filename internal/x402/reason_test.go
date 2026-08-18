package x402

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// A challenge says why this particular caller was refused.
//
// There was one sentence for everybody — "Sign in, or send a token from
// /token" — which is the right thing to tell a stranger and useless to somebody
// already signed in and out of credits. Telling an agent to sign in when it
// already has is how a refusal becomes a dead end: the one action it is offered
// is the one it has done.
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

	// No reason: the wording that names both ways in for somebody with no
	// account. It lives in one place, so an empty reason means "use it".
	anon, _ := body("")["error"].(string)
	if !strings.Contains(anon, "Sign in") {
		t.Errorf("the default challenge no longer tells a stranger how to start: %q", anon)
	}

	// A reason: used as given, and the accepts block still rides along, because
	// paying is the way forward that the words are pointing at.
	const mine = "This costs 2 credits and your balance is 0. Top up at https://micro.mu/account/topup"
	out := body(mine)
	if got, _ := out["error"].(string); got != mine {
		t.Errorf("error = %q, want %q", got, mine)
	}
	if _, ok := out["accepts"]; !ok {
		t.Error("a refusal with its own wording lost the price and the address to pay")
	}
}
