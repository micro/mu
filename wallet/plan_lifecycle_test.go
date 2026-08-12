package wallet

// A plan has to end when the paying does.
//
// Credits are a delivery: they arrive, they are yours, and cancelling does not
// claw them back. A plan is standing capacity — agents you keep, a rate you
// write at — so it has to be taken back when the subscription ends, or one
// month of Premium buys twenty-five agents for ever.
//
// The webhook handled two events and neither was a cancellation, which was
// harmless for exactly as long as nothing read Account.Plan. The moment the
// agent cap and the rate limit started reading it, not handling this became a
// way to keep what you stopped paying for.

import (
	"testing"

	"mu/internal/auth"
)

func planAccount(t *testing.T, id, plan string) *auth.Account {
	t.Helper()
	acc, err := auth.GetAccount(id)
	if err != nil {
		acc = &auth.Account{ID: id, Name: id}
		if err := auth.Create(acc); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}
	acc.Plan = plan
	if err := auth.UpdateAccount(acc); err != nil {
		t.Skipf("cannot update an account here: %v", err)
	}
	return acc
}

// TestACancelledPlanStopsGrantingCapacity.
func TestACancelledPlanStopsGrantingCapacity(t *testing.T) {
	const id = "plan-lifecycle-cancel"
	planAccount(t, id, "premium")

	clearPlan(id)

	acc, err := auth.GetAccount(id)
	if err != nil {
		t.Fatalf("account went missing: %v", err)
	}
	if acc.Plan != "" {
		t.Errorf("a cancelled subscription left the account on %q, so it keeps the "+
			"agents and the rate limit it stopped paying for", acc.Plan)
	}
	if got := PlanByID(acc.Plan); got.Agents != noPlan.Agents || got.PostsPerHour != noPlan.PostsPerHour {
		t.Errorf("after cancelling, the account is allowed %d agents and %d posts an "+
			"hour rather than %d and %d", got.Agents, got.PostsPerHour,
			noPlan.Agents, noPlan.PostsPerHour)
	}
}

// TestATopUpDoesNotEndAPlan — a one-off top-up goes through the same webhook
// branch as a subscription payment and carries no plan id. Writing that empty
// id would cancel somebody's plan for buying £5 of credits.
func TestATopUpDoesNotEndAPlan(t *testing.T) {
	const id = "plan-lifecycle-topup"
	planAccount(t, id, "pro")

	setPlan(id, "") // what a top-up's metadata contains

	acc, _ := auth.GetAccount(id)
	if acc.Plan != "pro" {
		t.Errorf("a top-up with no plan id left the account on %q, want pro", acc.Plan)
	}
}

// TestAPaymentPutsAnAccountOnItsPlan — the other direction, which is what the
// webhook was throwing away.
func TestAPaymentPutsAnAccountOnItsPlan(t *testing.T) {
	const id = "plan-lifecycle-buy"
	planAccount(t, id, "")

	setPlan(id, "premium")

	acc, _ := auth.GetAccount(id)
	if acc.Plan != "premium" {
		t.Errorf("paying for premium left the account on %q", acc.Plan)
	}
	if PlanByID(acc.Plan).Agents != PlanByID("premium").Agents {
		t.Error("the account is on premium and is not allowed what premium allows")
	}
}
