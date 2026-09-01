package agent

// Talking to the agent is charged, and the refusal says what to do.
//
// It was free, and it was the only model call on the instance that was: text_*
// is 1-6 credits, app_build 19, image_generate 19, web_search 3. On a hosted
// instance the agent is the largest thing it costs, and it was the one thing
// not priced.
//
// The objection that kept it free was real — an account that starts at zero
// cannot ask the question it signed up to ask — and the old answer to it was
// wrong: a daily grant of credits that cancelled the charge back out. The
// answer now is a welcome balance, granted once at signup. See
// account.WelcomeCredits.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/quota"
)

// An account with nothing is told what it needs and where to get it.
func TestAnEmptyAccountIsToldWhatItNeeds(t *testing.T) {
	// Charging turned on for this test rather than skipped when it is off. A
	// test that skips on the build it is meant to check reports success for a
	// gate nobody exercised — the same trap the mail gate's tests fell into,
	// where eighteen skipped quietly the day usernames became validated.
	was := quota.Enabled
	quota.Enabled = func() bool { return true }
	t.Cleanup(func() { quota.Enabled = was })

	if !quota.Metered(quota.OpAgentRun) {
		t.Fatal("agent_run is not metered even with charging on, so it is priced " +
			"at zero and the agent is free again")
	}

	// An owner first, so the account under test is not the instance's first
	// and therefore not its admin. Admins are exempt from charging, so a test
	// that skipped this would pass while exercising nothing — which is exactly
	// how TestASignedInCallerWithNoCreditsIsOfferedThePaymentPath came to be
	// green for the wrong reason.
	const owner = "chargeowner"
	if _, err := auth.GetAccount(owner); err != nil {
		if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "s"}); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(owner) }) //nolint:errcheck
	}

	const who = "chargebroke"
	if _, err := auth.GetAccount(who); err != nil {
		if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "s"}); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(who) }) //nolint:errcheck
	}
	acc, err := auth.GetAccount(who)
	if err != nil {
		t.Fatal(err)
	}
	// Not an admin: admins are exempt, and a test that passed because the
	// account happened to be the first one on the instance would be testing
	// nothing. This is the failure that bit TestASignedInCallerWithNoCredits.
	if acc.Admin {
		t.Fatal("the account under test is an admin, so the gate does not apply " +
			"to it and this test cannot say anything")
	}

	reason, ok := affordable(who)
	if ok {
		t.Fatalf("an account with %d credits was allowed a run costing %d",
			quota.BalanceOf(who), quota.OperationCost(quota.OpAgentRun))
	}
	// Both ways forward, named as places rather than routes — the same rule
	// service/mail's gate follows.
	for _, want := range []string{"credits", "Wallet"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the refusal does not mention %s: %s", want, reason)
		}
	}
	if strings.Contains(reason, "/wallet") {
		t.Errorf("the refusal names a route, which reads as a link and is not "+
			"one: %s", reason)
	}
}

// An unmetered instance never refuses.
//
// Self-hosted with no payments configured, a balance is a number that means
// nothing, and an agent that would not answer for want of it is the product
// broken by a feature nobody switched on.
func TestAnInstanceThatDoesNotChargeNeverRefuses(t *testing.T) {
	was := quota.Enabled
	quota.Enabled = func() bool { return false }
	t.Cleanup(func() { quota.Enabled = was })

	if reason, ok := affordable("anybody"); !ok {
		t.Errorf("an instance that does not charge refused a run: %s", reason)
	}
}

// The operation exists and has a price.
func TestTheAgentRunIsPriced(t *testing.T) {
	if quota.OpAgentRun != "agent_run" {
		t.Errorf("the operation is %q; quota.json names it agent_run", quota.OpAgentRun)
	}
	// Zero would mean the agent is free again, which is the thing this changed.
	// Only assert it where the instance charges at all.
	if got := quota.OperationCost(quota.OpAgentRun); got <= 0 {
		t.Errorf("agent_run costs %d, so the agent is free again", got)
	}
}
