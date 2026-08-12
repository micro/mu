package wallet

// A credit is a penny, on every plan and every top-up.
//
// It was not. Starter sold 500 credits for £5 and Pro sold 1,200 for £10 — a
// 20% discount on the one thing that has a marginal cost, so the plan that
// looked like the better business lost more the more it was used. Meanwhile
// /pricing advertised a free tier, £20 and £100 while /wallet still charged £5
// and £10: two pages, two lists, and nobody had read them together.
//
// The second half of that is fixed by construction — /pricing renders from
// SubscriptionPlans now, so there is one list. This is the first half. A
// discount is a decision somebody should have to make on purpose, and a label
// that disagrees with the price it labels is how the last one got in.

import (
	"fmt"
	"strings"
	"testing"
)

// TestACreditIsAPennyOnEveryPlan.
func TestACreditIsAPennyOnEveryPlan(t *testing.T) {
	for _, p := range SubscriptionPlans {
		if p.Credits != p.Price {
			t.Errorf("plan %q gives %d credits for %dp — a credit is a penny, so this "+
				"sells usage at %.0f%% of cost and loses more the more it is used",
				p.ID, p.Credits, p.Price, float64(p.Price)/float64(p.Credits)*100)
		}
	}
	for _, tier := range StripeTopupTiers {
		if tier.Credits != tier.Amount {
			t.Errorf("top-up %q gives %d credits for %dp — a credit is a penny",
				tier.Label, tier.Credits, tier.Amount)
		}
	}
}

// TestEveryPlanSaysWhatItCosts — the label is what a person clicks, so it has
// to be the price they are actually charged.
func TestEveryPlanSaysWhatItCosts(t *testing.T) {
	for _, p := range SubscriptionPlans {
		if !strings.Contains(p.Label, fmt.Sprintf("£%d/month", p.Price/100)) {
			t.Errorf("plan %q is £%d/month and its label says %q",
				p.ID, p.Price/100, p.Label)
		}
	}
}

// TestThePlansAreOrderedAndDistinct — the cards render in slice order, so the
// slice is the ordering. A cheaper plan below a dearer one is a bug you can
// only see by looking at the page.
func TestThePlansAreOrderedAndDistinct(t *testing.T) {
	if len(SubscriptionPlans) == 0 {
		t.Fatal("nothing is for sale")
	}
	seen := map[string]bool{}
	for i, p := range SubscriptionPlans {
		if seen[p.ID] {
			t.Errorf("two plans share the id %q", p.ID)
		}
		seen[p.ID] = true
		if p.Price <= 0 {
			t.Errorf("plan %q costs %dp — there is no free plan, and a plan priced "+
				"at nothing loses money in proportion to how much it is used",
				p.ID, p.Price)
		}
		if i > 0 && p.Price <= SubscriptionPlans[i-1].Price {
			t.Errorf("plan %q (%dp) comes after %q (%dp) and is not dearer",
				p.ID, p.Price, SubscriptionPlans[i-1].ID, SubscriptionPlans[i-1].Price)
		}
		if len(p.Features) == 0 {
			t.Errorf("plan %q lists nothing it gives you beyond the credits, and the "+
				"credits are the same price on every plan — so its card says nothing",
				p.ID)
		}
	}
}

// TestOnePlanIsRecommended — exactly one, or the highlight means nothing.
func TestOnePlanIsRecommended(t *testing.T) {
	n := 0
	for _, p := range SubscriptionPlans {
		if p.Featured {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d plans are featured — the pricing page highlights the featured "+
			"one, so there should be exactly one", n)
	}
}
