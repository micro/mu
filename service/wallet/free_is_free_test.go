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

func TestAFreeOperationIsNotMetered(t *testing.T) {
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

// And a paid one still asks, in the amount it costs rather than a floor.
func TestAPaidOperationStillChallenges(t *testing.T) {
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
