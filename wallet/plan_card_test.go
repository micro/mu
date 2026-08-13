package wallet

// Where a subscriber sees what they are paying, and how they stop.
//
// This product sold a monthly plan with neither. One place read acc.Plan — the
// pricing page, which is where you go to decide rather than to administer — and
// clearPlan had a single caller, the customer.subscription.deleted webhook. So
// a subscription ended only when it was cancelled inside Stripe, which the
// merchant can reach and the customer cannot: the two exits left were letting a
// card fail and filing a chargeback.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func payer(t *testing.T, id, plan, customer string) {
	t.Helper()
	auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}) //nolint:errcheck
	acc, err := auth.GetAccount(id)
	if err != nil {
		t.Skipf("cannot create an account here: %v", err)
	}
	acc.Plan, acc.Customer = plan, customer
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
}

// A subscriber is told what they are on and where to stop it.
func TestASubscriberSeesTheirPlanAndAWayOut(t *testing.T) {
	payer(t, "wallet-plan-pro", "pro", "cus_test")

	got := planCard("wallet-plan-pro")
	for _, want := range []string{"Pro", "£20", "Cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("the plan card never mentions %q:\n%s", want, got)
		}
	}
	// The button is the portal, which is where cancelling actually happens.
	if StripeEnabled() && !strings.Contains(got, `action="/wallet/stripe/portal"`) {
		t.Errorf("there is no route to manage the subscription:\n%s", got)
	}
	// And it says what cancelling costs, because "you lose your credits" is the
	// thing somebody fears and it is not true.
	if !strings.Contains(got, "credits you have already bought stay") &&
		!strings.Contains(got, "already bought stay") {
		t.Errorf("it does not say what happens to credits on cancelling:\n%s", got)
	}
}

// Pay as you go is a way of paying, not the absence of one, and the card should
// not read as a failure state.
func TestPayAsYouGoIsShownAsAPlanRatherThanAGap(t *testing.T) {
	payer(t, "wallet-plan-payg", "", "")

	got := planCard("wallet-plan-payg")
	if !strings.Contains(got, "Pay as you go") {
		t.Errorf("an account with no subscription is not told what it is on:\n%s", got)
	}
	if strings.Contains(got, "/wallet/stripe/portal") {
		t.Error("somebody with no subscription is offered a way to manage one")
	}
	if !strings.Contains(got, "/pricing") {
		t.Errorf("there is no route to a plan:\n%s", got)
	}
}

// The customer id is the handle on a subscription, so it is written once and
// then left alone: Stripe reuses a customer across purchases, and the first is
// the one holding the cards.
func TestTheStripeCustomerIsRecordedOnceAndNotOverwritten(t *testing.T) {
	const id = "wallet-plan-customer"
	payer(t, id, "", "")

	setCustomer(id, "cus_first")
	if acc, _ := auth.GetAccount(id); acc.Customer != "cus_first" {
		t.Fatalf("the customer was not recorded: %q", acc.Customer)
	}

	setCustomer(id, "cus_second")
	if acc, _ := auth.GetAccount(id); acc.Customer != "cus_first" {
		t.Errorf("a later checkout repointed the account at %q, which may be the "+
			"emptier of two customers", acc.Customer)
	}

	// Anything that is not a customer id is not written. A malformed webhook
	// should not be able to strand somebody's billing.
	payer(t, id+"-junk", "", "")
	setCustomer(id+"-junk", "sub_notacustomer")
	if acc, _ := auth.GetAccount(id + "-junk"); acc.Customer != "" {
		t.Errorf("a non-customer id was stored: %q", acc.Customer)
	}
}

// Somebody with no billing record gets a reason rather than a blank page.
func TestNoBillingRecordSaysSoRatherThanFailingQuietly(t *testing.T) {
	payer(t, "wallet-plan-nobilling", "", "")

	_, err := CustomerFor("wallet-plan-nobilling")
	if err == nil {
		t.Fatal("an account that has never paid resolved to a customer")
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Errorf("the reason does not say what is missing: %v", err)
	}
}

// A plan switched inside Stripe's own portal has to reach the account.
//
// Otherwise the portal is a way for a customer to desync silently: move Pro to
// Scale and keep five agents while being charged a hundred pounds, or the other
// way and keep twenty-five while paying twenty. Neither is a state anybody would
// report, which is what makes it worth a test rather than a comment.
func TestAPlanIsIdentifiedByWhatItCostsNotByItsMetadata(t *testing.T) {
	for _, p := range SubscriptionPlans {
		if got := planForPrice(p.Price); got != p.ID {
			t.Errorf("a subscription at %d pence resolves to %q, want %q", p.Price, got, p.ID)
		}
	}
	// A price that is no plan of ours resolves to nothing rather than to the
	// first one in the list.
	if got := planForPrice(1); got != "" {
		t.Errorf("an unknown price resolved to the %q plan", got)
	}

	// Distinct prices are what makes this work. Two plans at one price would be
	// ambiguous here — and indistinguishable on the pricing page too.
	seen := map[int]string{}
	for _, p := range SubscriptionPlans {
		if prev, dup := seen[p.Price]; dup {
			t.Errorf("%s and %s both cost %d, so a switch between them cannot be read "+
				"back from the subscription", prev, p.ID, p.Price)
		}
		seen[p.Price] = p.ID
	}
}
