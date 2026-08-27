package test

// Signing up must not take away the way to pay.
//
// The payment gate asked one question — is there a valid token — and treated
// "yes" as the end of it. So the x402 challenge, which carries the price, the
// asset, the chain and the address to pay, only ever reached a caller with no
// account. Signing in put you in the credits lane permanently: an agent that
// ran out was refused with a sentence about a web page, over a protocol it
// cannot browse, having never been told this server takes payment per call.
//
// The result was backwards. Anonymous, an agent got ten free trial calls and
// then a machine-readable price. Signed in — which is what the MCP registry
// listing leads people to do — it got a dead end. Finding the server and then
// registering with it made it less usable.
//
// What payer decides is tested for real in internal/server/pay_test.go. What
// is held here is the wiring between it and the challenge, which needs a live
// HTTP stack to observe and so is checked in the source, the same way
// TestThePaymentGateAsksWhetherTheToolCostsAnything is.

import (
	"os"
	"strings"
	"testing"
)

func gateSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(at("internal/server/serve.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheGateAsksWhoIsPayingNotWhetherTheyExist(t *testing.T) {
	gate := gateSource(t)

	if strings.Contains(gate, "} else if err := auth.ValidateToken(token); err != nil {") {
		t.Error("the payment gate is back to asking only whether a token is valid, " +
			"so a signed-in caller with no credits never sees the challenge that " +
			"would let it pay")
	}
	if !strings.Contains(gate, "payer(r, token, op)") {
		t.Error("the gate no longer asks who is paying and whether they can")
	}
}

// The reason has to reach the caller, or every refusal reads as though they
// were a stranger — which is advice a signed-in agent cannot act on.
func TestTheChallengeCarriesTheReasonThroughToTheCaller(t *testing.T) {
	// The reason is the last argument, whatever the ones before it are. Pinning
	// the whole call meant this failed when the discovery listing between them
	// was dropped, which had nothing to do with reasons reaching callers.
	if !strings.Contains(gateSource(t), "x402.WritePaymentRequired(w, op, resource") ||
		!strings.Contains(gateSource(t), ", reason)") {
		t.Error("the gate no longer passes a reason to the challenge")
	}
}
