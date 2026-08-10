package wallet

// A tool priced at zero must not ask anyone for money.
//
// BuildPaymentRequirements floored the cost at one credit, so the four tools
// deliberately priced at zero — news search, web fetch, quran search, video
// search, all things nothing bills us for — answered an anonymous caller with
// a demand for USDC on Base. The free tier existed in the price list and was
// unreachable at the door.
//
// That is the whole no-signup claim: an agent finds the endpoint mid-task,
// tries a tool, and it either works or it does not. One that refuses every
// unpaid call never gets tried twice.

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Free and open are different questions.
//
// web_fetch and video_search cost nothing and are still not for strangers:
// fetching any URL a caller names is a request this server makes on their
// behalf, and the YouTube quota is a shared 10,000 units a day. Both are
// rationed per account, and an anonymous caller cannot be rationed. They are
// marked AccountOnly, which refuses them at the MCP layer with a 401 rather
// than offering a payment that would unlock nothing — the price was never what
// was standing in the way.
//
// So "priced at zero" must not be read as "open", anywhere.
func TestFreeDoesNotMeanOpen(t *testing.T) {
	withPayments(t) // so "not metered" means priced at zero, not payments off
	for _, gated := range []string{OpWebFetch, OpVideoSearch} {
		if Metered(gated) {
			t.Errorf("%s is metered, so an anonymous caller is asked to pay for "+
				"something paying does not unlock", gated)
		}
	}
}

func TestAFreeOperationIsNotMetered(t *testing.T) {
	withPayments(t) // the question is the price, not whether payments exist
	for _, free := range []string{OpNewsSearch, OpWebFetch, OpQuranSearch} {
		if GetOperationCost(free) != 0 {
			t.Fatalf("%s is no longer priced at zero — this test is about the ones that are", free)
		}
		if Metered(free) {
			t.Errorf("%s reads as metered, so the gate will charge for it", free)
		}
		if reqs := BuildPaymentRequirements(free, "https://example.test/mcp"); len(reqs) != 0 {
			t.Errorf("%s produced %d payment requirements: a free tool priced at %s",
				free, len(reqs), reqs[0].MaxAmountRequired)
		}
	}
}

// withPayments turns this instance into one that can charge, which is what
// "metered" is relative to.
func withPayments(t *testing.T) {
	t.Helper()
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_x")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_x")
	if !PaymentsEnabled() {
		t.Skip("payments cannot be enabled in this environment")
	}
}

// Nothing is metered where nothing can be charged.
//
// A self-hosted instance with no Stripe and no x402 has no meter, no price and
// nobody to bill — CheckQuota has always said so. The gates in front of it did
// not ask, so a fresh install refused an anonymous caller with "this call is
// metered" for weather, which is the first thing anybody tries.
func TestWithoutPaymentsNothingIsMetered(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "")
	if PaymentsEnabled() {
		t.Skip("this instance is configured to take payments")
	}
	for _, op := range []string{OpWebSearch, OpImageGenerate, OpAgentQuery} {
		if Metered(op) {
			t.Errorf("%s reads as metered on an instance that cannot charge", op)
		}
	}
}

// And a paid one still asks, in the amount it costs rather than a floor.
func TestAPaidOperationStillChallenges(t *testing.T) {
	withPayments(t)
	if !Metered(OpWebSearch) {
		t.Fatal("web search stopped being metered, so nobody is charged for Brave")
	}
	reqs := BuildPaymentRequirements(OpWebSearch, "https://example.test/mcp")
	if len(reqs) == 0 {
		t.Skip("no payment assets configured on this instance")
	}
	if reqs[0].MaxAmountRequired == "" || reqs[0].MaxAmountRequired == "0" {
		t.Errorf("a paid tool asks for %q", reqs[0].MaxAmountRequired)
	}
}

// The gate has to be able to say "there is nothing to pay". One that can only
// say "pay" is how a free tool ends up behind a paywall.
func TestTheChallengeReportsWhetherItWroteOne(t *testing.T) {
	w := httptest.NewRecorder()
	if WritePaymentRequired(w, OpNewsSearch, "https://example.test/mcp") {
		t.Error("a free operation produced a 402 challenge")
	}
	if w.Code != 200 || strings.Contains(w.Body.String(), "x402Version") {
		t.Errorf("something was written for a free operation: %d %s", w.Code, w.Body.String())
	}

	paid := httptest.NewRecorder()
	if !WritePaymentRequired(paid, OpWebSearch, "https://example.test/mcp") {
		if X402Enabled() {
			t.Error("a paid operation wrote no challenge on an x402 instance")
		}
		return
	}
	if paid.Code != 402 {
		t.Errorf("a paid operation answered %d, want 402", paid.Code)
	}
}
