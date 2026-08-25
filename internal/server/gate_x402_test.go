package server

// Paying is not a reason to be refused.
//
// x402's whole claim is that an agent can call a priced tool with no account:
// the call answers 402 with a challenge, the agent pays, retries, and gets the
// answer. VerifyAndSettle takes the money at the door, before the tool runs.
//
// Then the gateway asks the wallet whether this caller may proceed, and the
// caller is a wallet address rather than a username — so there is no account,
// which is the point — and CheckQuota answers "account not found". The gateway
// turns that into a refusal. The payment has settled and the caller has
// nothing.
//
// This was live for every scoped priced service, and binding the caller on
// priced tools spread it to web_search, which is the tool the README uses to
// demonstrate x402.

import (
	"sync"
	"testing"

	"mu/internal/service"
)

// wireOnce stands the server's wiring up exactly once for this binary.
//
// wireHooks registers services and starts the background loops that subscribe
// to them, so calling it twice has a second registration racing the first
// one's goroutines — which the detector reports against internal/service and
// internal/event rather than against the test that did it.
var wireOnce = sync.OnceFunc(wireHooks)

// TestAPaidWalletIsNotRefusedByTheGate.
func TestAPaidWalletIsNotRefusedByTheGate(t *testing.T) {
	wireOnce()
	if service.Gate.Allow == nil {
		t.Fatal("the gate is not wired, so this proves nothing")
	}

	charge, err := service.Gate.Allow("x402:0x1111111111111111111111111111111111111111", "web_search")
	if err != nil {
		t.Fatalf("a caller who has already paid in USDC was refused: %v — the payment "+
			"settled before the tool ran, so this is money taken for nothing", err)
	}
	if charge {
		t.Error("a paid wallet was also marked chargeable, which would bill the same " +
			"call twice: once in USDC at the door and once in credits it does not have")
	}
}

// TestAnOrdinaryAccountIsStillAsked — the exemption above must not be a hole
// somebody falls through by accident.
func TestAnOrdinaryAccountIsStillAsked(t *testing.T) {
	wireOnce()
	if _, err := service.Gate.Allow("definitely_no_account", "web_search"); err == nil {
		t.Error("an unknown account was let through a priced call, so the wallet " +
			"identity exemption is catching more than wallet identities")
	}
}
