package test

// There is one way to charge, and this is what keeps it that way.
//
// There were four. quota.ConsumeQuota, quota.ConsumeWith, app.Charge and a
// charge() of service/mail's own, spread over twenty-nine call sites, and which
// one a service used was historical accident rather than decision. That is how
// the same operation came to be charged twice through one door and not at all
// through another, how a page and a tool disagreed about what a call cost, and
// why nobody could answer "what does this cost" without reading five files.
//
// The count is the thing that matters. Every one of those was defensible on its
// own and the pile was not, and a pile grows one defensible addition at a time —
// so the rule is enforced rather than written down and hoped for.
//
// quota.Charge is the only function that may move credits. Everything else asks
// quota.CheckQuota whether it may proceed. If a service needs something neither
// of those does, the answer is to change them, not to add a third.

import (
	"strings"
	"testing"
)

func TestOneWayToCharge(t *testing.T) {
	// Names that used to take payment, or that look like they might. Any of
	// them reappearing is the pile starting again.
	banned := []string{
		"ConsumeQuota(",
		"ConsumeWith(",
		"app.Charge(",
		"DeductCredits(",
	}

	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		// The wallet is where credits actually live, and quota is the one door
		// to it. Both are allowed to say these words; nothing else is.
		if strings.Contains(path, "internal/quota/") ||
			strings.Contains(path, "account/credits.go") {
			return
		}
		body := stripComments(src)
		for _, name := range banned {
			if strings.Contains(body, name) {
				t.Errorf("%s calls %s — quota.Charge is the only way to take "+
					"payment. There were four of these once, over twenty-nine "+
					"call sites, and two of them disagreed about what the same "+
					"operation cost.", path, strings.TrimSuffix(name, "("))
			}
		}
	})
}

// And nothing outside the wallet reaches past quota to the ledger.
func TestOnlyQuotaTouchesTheLedger(t *testing.T) {
	walkGo(t, func(path, src string) {
		if strings.HasSuffix(path, "_test.go") ||
			strings.Contains(path, "internal/quota/") ||
			strings.Contains(path, "account/") {
			return
		}
		if strings.Contains(stripComments(src), "quota.Deduct(") {
			t.Errorf("%s calls quota.Deduct directly, going around the price "+
				"check and the usage record", path)
		}
	})
}
