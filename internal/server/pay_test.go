package server

// Signing up must not take away the way to pay.
//
// The gate asked whether a token was valid and treated "yes" as the end of it,
// so the x402 challenge only ever reached callers with no account. An agent
// that signed up through the MCP registry and then ran out of credits was
// refused with a sentence about a web page, over a protocol it cannot browse,
// having never been told this server takes payment per call.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/quota"
)

// broke stands up an account with a token and a balance, and returns the raw
// token. Charging is switched on for the duration, because an instance that
// cannot charge meters nothing and the question does not arise.
func broke(t *testing.T, id string, balance int) string {
	t.Helper()

	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("could not create %s: %v", id, err)
	}
	_, secret, err := auth.CreateToken(id, "agent: test", nil, time.Time{})
	if err != nil {
		t.Fatalf("could not issue a token for %s: %v", id, err)
	}

	balances := map[string]int{id: balance}
	prevEnabled, prevBalance := quota.Enabled, quota.Balance
	quota.Enabled = func() bool { return true }
	quota.Balance = func(account string) int { return balances[account] }
	t.Cleanup(func() { quota.Enabled, quota.Balance = prevEnabled, prevBalance })

	// And today's free allowance spent, so the balance is the only thing left
	// deciding. Every account gets quota.DailyQuota credits a day whether or
	// not it has ever paid — see internal/quota/allowance.go — so without this
	// an account with a balance of zero is not broke, it is new, and these
	// tests are about what happens to somebody who has run out.
	quota.ResetAllowances()
	t.Cleanup(quota.ResetAllowances)
	for quota.FreeCreditsLeft(id) > 0 && quota.SpendFreeCredits(id, 1) {
	}

	return secret
}

func ask(token string) *http.Request {
	r := httptest.NewRequest("POST", "/mcp", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// The case the whole change is about: known caller, no money.
func TestASignedInCallerWithNoCreditsIsOfferedThePaymentPath(t *testing.T) {
	token := broke(t, "skint-agent", 0)

	who, blocked, reason := payer(ask(token), token, quota.OpWebSearch)

	if !blocked {
		t.Fatal("an account with no credits was let through a metered call")
	}
	if who != "skint-agent" {
		t.Errorf("the refusal is filed under %q, not the account that made it", who)
	}
	if reason == "" {
		t.Fatal("a signed-in caller gets the stranger's wording, which tells it to sign in")
	}
	if !strings.Contains(reason, "X-PAYMENT") {
		t.Errorf("the one way forward an agent can take is not mentioned: %q", reason)
	}
	if !strings.Contains(reason, "balance is 0") {
		t.Errorf("the reason does not say what is wrong: %q", reason)
	}
}

// With credits, nothing changes: no challenge, and the call proceeds to be
// charged the way it always was.
func TestASignedInCallerWithCreditsIsNotStopped(t *testing.T) {
	token := broke(t, "funded-agent", 500)

	who, blocked, reason := payer(ask(token), token, quota.OpWebSearch)

	if blocked {
		t.Fatalf("an account with credits was refused: %q", reason)
	}
	if who != "funded-agent" {
		t.Errorf("the caller is %q", who)
	}
}

// And a stranger still gets the challenge's own wording, which names both ways
// in. Empty means "use the default" — the sentence lives in one place.
func TestAStrangerStillGetsTheStandardChallenge(t *testing.T) {
	who, blocked, reason := payer(ask(""), "", quota.OpWebSearch)

	if !blocked {
		t.Fatal("an anonymous caller was let through a metered call")
	}
	if reason != "" {
		t.Errorf("a stranger gets bespoke wording rather than the standard one: %q", reason)
	}
	if who != "guest" {
		t.Errorf("an anonymous refusal is filed under %q", who)
	}
}

// A token that is not a token is a stranger, not an error.
func TestAnUnusableTokenIsTreatedAsAStranger(t *testing.T) {
	_, blocked, reason := payer(ask("not-a-real-token"), "not-a-real-token", quota.OpWebSearch)
	if !blocked {
		t.Fatal("a bad token got through")
	}
	if reason != "" {
		t.Errorf("a bad token produced a balance-shaped refusal: %q", reason)
	}
}
