package home

// The page that says what this costs, which did not exist.

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"mu/account"
	"mu/internal/app"
	"mu/internal/quota"
)

// takesPayment makes this instance one that charges, so the page under test is
// the one a hosted visitor sees rather than the self-hosted one.
func takesPayment(t *testing.T) {
	t.Helper()
	// The environment only, and nothing written to the settings store.
	//
	// settings.Get answers from the environment before its own file, so an
	// exported key is enough — and saving and restoring the stored one is how
	// this leaks. Written that way first, it read the value it had just
	// exported and wrote it into the store on cleanup, where t.Setenv cannot
	// reach it: the next test in this file, the one about an instance that
	// charges nothing, then found an instance that charges. Same fault as
	// clearProviders in agent/, reproduced within the hour.
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_pricing")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_pricing")
}

func pricingPage(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	PricingHandler(rec, httptest.NewRequest("GET", "/pricing", nil))
	if rec.Code != 200 {
		t.Fatalf("/pricing answered %d", rec.Code)
	}
	return rec.Body.String()
}

// The headline price is on the page, and it is the price this instance charges.
//
// The table is sorted cheapest first, so the one number nearly everybody is
// here for sits in the middle of twenty rows looking like a pollen forecast.
// Saying it in the paragraph is the answer — and saying it from the price list
// rather than from a literal is what stops the sentence going stale the day an
// operator changes CREDIT_COST_AGENT_RUN.
func TestPricingLeadsWithWhatAQuestionCosts(t *testing.T) {
	takesPayment(t)
	body := pricingPage(t)

	cost := quota.OperationCost(quota.OpAgentRun)
	if cost <= 0 {
		t.Fatalf("agent_run costs %d, so this test cannot say anything", cost)
	}
	want := "the agent is " + strconv.Itoa(cost) + "¢"
	if !strings.Contains(body, want) {
		t.Errorf("the page does not say what a question costs (%q): %s", want, clipPage(body))
	}
	// And the table under it, which is the shared one — it had no caller
	// outside its own test while three comments described the page rendering
	// it.
	if !strings.Contains(body, "Image generation") {
		t.Error("the cost table is not on the pricing page")
	}
}

// What you get before you pay anything, said as money and read from the
// constant that decides it.
func TestPricingSaysWhatYouGetToStart(t *testing.T) {
	takesPayment(t)
	body := pricingPage(t)

	if account.WelcomeCredits != 100 {
		t.Skip("the welcome balance is no longer a round dollar; this test asserts the wording of one")
	}
	if !strings.Contains(body, "$1 of credit") {
		t.Errorf("the page does not say what a new account gets: %s", clipPage(body))
	}
	for _, want := range []string{`href="/signup"`, `href="/wallet/topup"`, "$5"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not carry %s — a price with no way to pay it is an advert", want)
		}
	}
}

// An instance nobody can pay charges nobody, and says so.
//
// The default in a test is no Stripe keys, which is also the default on a box
// somebody runs themselves. A table of prices there is a page of numbers that
// will never be applied to anyone, and it invites a self-hoster to worry about
// a bill that does not exist.
func TestPricingOnAnInstanceThatChargesNothing(t *testing.T) {
	if account.PaymentsEnabled() {
		t.Skip("this box has payment keys configured, so there is no unmetered case to check")
	}
	body := pricingPage(t)

	if strings.Contains(body, "Image generation") || strings.Contains(body, "¢") {
		t.Errorf("an unmetered instance is showing a price list: %s", clipPage(body))
	}
	if !strings.Contains(strings.ToLower(body), "nothing") {
		t.Errorf("the page does not say that nothing is charged: %s", clipPage(body))
	}
	if strings.Contains(body, `href="/wallet/topup"`) {
		t.Error("an instance that cannot take a payment is offering a top-up")
	}
}

// The footer carries it, because a signed-out visitor is the whole audience for
// a price.
//
// The footer's own comment argued for this link while the line below it had
// four links and no Pricing in it, and /pricing was a 404 in production. An
// argument that outlives its page reads as done.
func TestTheFooterCarriesThePrice(t *testing.T) {
	if !strings.Contains(app.FooterLinks(), `href="/pricing"`) {
		t.Errorf("the footer does not carry the price: %s", app.FooterLinks())
	}
}

func clipPage(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
