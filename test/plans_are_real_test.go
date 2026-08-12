package test

// A plan card must not promise what nothing enforces.
//
// The pricing page sold "1 agent", "5 agents", "25 agents", "Higher rate
// limits" and "Send mail, SMS and WhatsApp" as the differences between the
// tiers — the entire reason to pick one, since credits are 1p on all of them.
// None of it existed. There was no field on an account saying what plan it was
// on, nothing counted agents, and the post rate limit varied by whether an
// account was an admin, approved, or under a day old, and by nothing anybody
// could pay for.
//
// So a subscriber got exactly what a non-subscriber got, plus credits they
// could have bought as a top-up. This is the test that says otherwise.

import (
	"testing"

	"mu/agent"
	"mu/internal/auth"
	"mu/wallet"
)

// TestEveryPlanSellsSomethingBesidesCredits — if two plans differ only in
// credits, the dearer one is a top-up with extra steps.
func TestEveryPlanSellsSomethingBesidesCredits(t *testing.T) {
	plans := wallet.Plans()
	for i, p := range plans {
		if p.Agents <= 0 {
			t.Errorf("plan %q allows %d agents, so its card cannot say what it runs",
				p.ID, p.Agents)
		}
		if p.PostsPerHour <= 0 {
			t.Errorf("plan %q has no post rate, so 'higher rate limits' means nothing",
				p.ID)
		}
		if i == 0 {
			continue
		}
		prev := plans[i-1]
		if p.Agents == prev.Agents && p.PostsPerHour == prev.PostsPerHour {
			t.Errorf("plan %q costs more than %q and allows exactly the same things — "+
				"the only difference is credits, which are 1p on both, so it sells "+
				"nothing a top-up does not", p.ID, prev.ID)
		}
	}
}

// TestAPlanBuysMoreThanNoPlan — the cheapest plan has to beat not subscribing
// at something other than the credits, or there is no reason to subscribe.
func TestAPlanBuysMoreThanNoPlan(t *testing.T) {
	plans := wallet.Plans()
	if len(plans) == 0 {
		t.Fatal("nothing is for sale")
	}
	none := wallet.PlanByID("")
	top := plans[len(plans)-1]
	if top.Agents <= none.Agents && top.PostsPerHour <= none.PostsPerHour {
		t.Errorf("the dearest plan (%s) allows no more than no plan at all", top.ID)
	}
}

// TestAnUnknownPlanIsNotAZeroAllowance — a subscription Stripe knows about and
// this build does not must not leave somebody unable to do anything.
func TestAnUnknownPlanIsNotAZeroAllowance(t *testing.T) {
	for _, id := range []string{"", "starter", "enterprise-2029", "nonsense"} {
		p := wallet.PlanByID(id)
		if p.Agents <= 0 || p.PostsPerHour <= 0 {
			t.Errorf("plan id %q resolves to agents=%d posts=%d — an unrecognised plan "+
				"must fall back to what everybody gets, not to nothing",
				id, p.Agents, p.PostsPerHour)
		}
	}
}

// TestThePlanRateLimitIsWiredIntoTheLimiter — the numbers existing is half of
// it. This is the half that made them true.
func TestThePlanRateLimitIsWiredIntoTheLimiter(t *testing.T) {
	orig := auth.PlanPostsPerHour
	t.Cleanup(func() { auth.PlanPostsPerHour = orig })
	auth.PlanPostsPerHour = func(plan string) int { return wallet.PlanByID(plan).PostsPerHour }

	pro := wallet.PlanByID("pro")
	plain := auth.PostLimitFor(auth.Account{ID: "plain"})
	paid := auth.PostLimitFor(auth.Account{ID: "paid", Plan: "pro"})
	if paid <= plain {
		t.Errorf("a Pro account may post %d an hour and an account with no plan may "+
			"post %d — the page sells 'higher rate limits'", paid, plain)
	}
	if paid != pro.PostsPerHour {
		t.Errorf("Pro allows %d posts an hour and the limiter gives it %d",
			pro.PostsPerHour, paid)
	}
}

// TestTheAgentCapIsWiredIntoCreation — same again for the agent count, which is
// the first line on every card.
func TestTheAgentCapIsWiredIntoCreation(t *testing.T) {
	orig := agent.PlanAgents
	t.Cleanup(func() { agent.PlanAgents = orig })

	const owner = "plan-agent-cap"
	agent.PlanAgents = func(string) int { return 1 }

	// Whatever this account already has, one more than its allowance is
	// refused. Creating the first is the setup, not the assertion.
	if n := len(agent.Agents(owner)); n == 0 {
		if _, _, err := agent.CreateAgent(owner, "First", agent.External, "", "", nil, false); err != nil {
			t.Skipf("cannot create an agent in this environment: %v", err)
		}
		t.Cleanup(func() {
			for _, a := range agent.Agents(owner) {
				agent.RemoveAgent(owner, a.ID) //nolint:errcheck
			}
		})
	}
	if _, _, err := agent.CreateAgent(owner, "Second", agent.External, "", "", nil, false); err == nil {
		t.Error("an account allowed 1 agent created a second one, so the number on " +
			"the pricing card is decoration")
	}
}

// TestNoLimitWhenNothingIsBeingSold — a self-hosted instance with no billing
// linked in must not inherit a cap it never agreed to.
func TestNoLimitWhenNothingIsBeingSold(t *testing.T) {
	orig := agent.PlanAgents
	t.Cleanup(func() { agent.PlanAgents = orig })
	agent.PlanAgents = nil

	const owner = "plan-agent-selfhost"
	a, _, err := agent.CreateAgent(owner, "Selfhosted", agent.External, "", "", nil, false)
	if err != nil {
		t.Fatalf("an unmetered instance refused an agent: %v", err)
	}
	agent.RemoveAgent(owner, a.ID) //nolint:errcheck
}
