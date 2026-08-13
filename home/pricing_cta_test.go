package home

// What the button on a plan card does, which depends on who is reading it.
//
// Every card said "Get Pro" and pointed at /signup, for everybody. So an account
// that hit its agent limit was told to come here to run more, came here, clicked
// Pro, and was handed a signup form for the account it was already signed into.
// The one route the limit message exists to open was the one that dead-ended,
// and nothing could notice because the page renders the same bytes either way.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/wallet"
)

// signedIn returns a request carrying a session for an account on the given
// plan.
func signedIn(t *testing.T, id, plan string) *http.Request {
	t.Helper()
	auth.Create(&auth.Account{ID: id, Name: id, Secret: "test-secret"}) //nolint:errcheck
	acc, err := auth.GetAccount(id)
	if err != nil {
		t.Skipf("cannot create an account here: %v", err)
	}
	acc.Plan = plan
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatalf("setting the plan: %v", err)
	}
	t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck

	sess, err := auth.CreateSession(id)
	if err != nil {
		t.Fatalf("could not sign in as %s: %v", id, err)
	}
	r := httptest.NewRequest("GET", "/pricing", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return r
}

// A visitor with no account is being asked to start, which is the only thing
// they can do.
func TestSignedOutEveryCardOffersSignup(t *testing.T) {
	r := httptest.NewRequest("GET", "/pricing", nil)

	for _, p := range append([]wallet.SubscriptionPlan{wallet.PlanByID("")}, wallet.Plans()...) {
		got := cta(r, p.ID)
		if !strings.Contains(got, `href="/signup"`) {
			t.Errorf("the %s card does not offer signup to a visitor: %s", p.Name, got)
		}
		if strings.Contains(got, "/wallet/stripe/subscribe") {
			t.Errorf("the %s card offers to charge somebody with no account: %s", p.Name, got)
		}
	}
}

// Signed in, the card for a plan you are not on is the thing that changes it.
func TestSignedInAPlanCardSubscribesRatherThanSigningYouUpAgain(t *testing.T) {
	plans := wallet.Plans()
	if len(plans) == 0 {
		t.Skip("no plans configured")
	}
	r := signedIn(t, "pricing-cta-none", "")

	got := cta(r, plans[0].ID)
	if strings.Contains(got, `href="/signup"`) {
		t.Errorf("a signed-in account is sent to signup: %s", got)
	}
	if !strings.Contains(got, `action="/wallet/stripe/subscribe"`) {
		t.Errorf("the card does not subscribe: %s", got)
	}
	if !strings.Contains(got, `value="`+plans[0].ID+`"`) {
		t.Errorf("the form does not name the plan it is for: %s", got)
	}
}

// The plan you are already on says so rather than selling it to you again.
func TestTheCardForYourOwnPlanSaysSo(t *testing.T) {
	plans := wallet.Plans()
	if len(plans) == 0 {
		t.Skip("no plans configured")
	}
	r := signedIn(t, "pricing-cta-onplan", plans[0].ID)

	got := cta(r, plans[0].ID)
	if !strings.Contains(got, "Your plan") {
		t.Errorf("the card for the plan this account is on reads: %s", got)
	}
	if strings.Contains(got, "subscribe") {
		t.Errorf("it offers to subscribe to the plan already held: %s", got)
	}

	// And pay-as-you-go is what somebody with no plan is on.
	none := signedIn(t, "pricing-cta-payg", "")
	if got := cta(none, ""); !strings.Contains(got, "Your plan") {
		t.Errorf("an account with no subscription is not shown as pay-as-you-go: %s", got)
	}
}

// Leaving a plan is a cancellation, and a pricing card is not where that should
// happen in one click.
func TestDroppingToPayAsYouGoGoesThroughTheWallet(t *testing.T) {
	plans := wallet.Plans()
	if len(plans) == 0 {
		t.Skip("no plans configured")
	}
	r := signedIn(t, "pricing-cta-drop", plans[0].ID)

	got := cta(r, "")
	if strings.Contains(got, "subscribe") {
		t.Errorf("a subscription can be cancelled from a pricing card: %s", got)
	}
	if !strings.Contains(got, `href="/wallet"`) {
		t.Errorf("there is no route to manage the subscription: %s", got)
	}
}

// Arriving from a limit, the first question is which of these you already have.
func TestTheSignedInPageSaysWhichPlanYouAreOn(t *testing.T) {
	r := signedIn(t, "pricing-cta-banner", "")
	rr := httptest.NewRecorder()
	PricingHandler(rr, r)

	body := rr.Body.String()
	if !strings.Contains(body, "You are on") {
		t.Error("the page never says which plan this account is on")
	}
	// The agent number, because that is what somebody who just met the agent
	// limit came here to find out.
	if !strings.Contains(body, "agent") {
		t.Error("it does not say how many agents the current plan runs")
	}

	// And a visitor is not told they are on anything.
	anon := httptest.NewRecorder()
	PricingHandler(anon, httptest.NewRequest("GET", "/pricing", nil))
	if strings.Contains(anon.Body.String(), "You are on") {
		t.Error("a signed-out visitor is told which plan they are on")
	}
}
