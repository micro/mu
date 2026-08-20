package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mu/internal/quota"
)

// jsonRequest is a guest asking for JSON — no session, no account, nobody to
// bill. The shape of the caller that used to be turned away for free calls.
func jsonRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/anything", nil)
	r.Header.Set("Accept", "application/json")
	return r
}

// realPrices loads the repository's own quota.json, so these tests read the
// prices the product actually ships rather than the unpriced default of 1.
// Without it "is this operation free" is answered by an empty table, and a test
// that skips because it could not find a price is a test that checks nothing.
func realPrices(t *testing.T) {
	t.Helper()
	if err := quota.LoadFromTree(); err != nil {
		t.Fatalf("could not load quota.json: %v", err)
	}
}

// withCharging switches this instance between "can bill somebody" and "cannot",
// which is the only thing that decides whether a priced operation is metered.
func withCharging(t *testing.T, on bool) {
	t.Helper()
	prev := quota.Enabled
	quota.Enabled = func() bool { return on }
	t.Cleanup(func() { quota.Enabled = prev })
}

// TestAGuestGetsAFreeCallOnAFreeInstance is the whole point of the gate.
//
// On a self-hosted instance nothing is metered, so there is nobody who has to
// be named and no reason to collect an account. Weather, news search, web
// search and web fetch each refused this caller before the rule was shared.
func TestAGuestGetsAFreeCallOnAFreeInstance(t *testing.T) {
	withCharging(t, false)

	for _, op := range []string{quota.OpWeatherForecast, quota.OpWebSearch, quota.OpNewsSearch, quota.OpWebFetch} {
		w := httptest.NewRecorder()
		id, ok := BillableCaller(w, jsonRequest(), op)
		if !ok {
			t.Errorf("%s refused a guest on an instance that cannot charge (HTTP %d)", op, w.Code)
		}
		if id != "" {
			t.Errorf("%s named %q as the caller when nobody was signed in", op, id)
		}
	}
}

// TestAFreeOperationIsFreeEvenWhenCharging — an operation priced at zero costs
// this instance nothing to run, so it needs nobody to bill even where the
// instance bills for other things.
func TestAFreeOperationIsFreeEvenWhenCharging(t *testing.T) {
	realPrices(t)
	withCharging(t, true)

	// news_search and web_fetch are priced at 0 in quota.json: they touch only
	// this instance's own storage and bandwidth.
	for _, op := range []string{quota.OpNewsSearch, quota.OpWebFetch} {
		if cost := quota.OperationCost(op); cost != 0 {
			t.Errorf("%s costs %d in quota.json — it reads this instance's own "+
				"storage, so pricing it means this rule needs revisiting", op, cost)
			continue
		}
		w := httptest.NewRecorder()
		if _, ok := BillableCaller(w, jsonRequest(), op); !ok {
			t.Errorf("%s refused a guest for an operation that costs nothing (HTTP %d)", op, w.Code)
		}
	}
}

// TestAMeteredCallStillNeedsAnAccount is the other half, and the half that must
// not have been loosened: where this instance really does pay per call, it
// really does need somebody to bill.
func TestAMeteredCallStillNeedsAnAccount(t *testing.T) {
	realPrices(t)
	withCharging(t, true)

	if quota.OperationCost(quota.OpWebSearch) == 0 {
		t.Fatal("web_search is priced at zero — Brave is paid for per call, so " +
			"a price of nothing means somebody is being given away")
	}
	w := httptest.NewRecorder()
	if _, ok := BillableCaller(w, jsonRequest(), quota.OpWebSearch); ok {
		t.Fatal("a metered call let an anonymous caller through — nobody could be billed for it")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("refusal was HTTP %d, want 401 — a guest needs to know to sign in", w.Code)
	}
}

// app.Charge is gone. It was a second way to take payment sitting beside
// quota.ConsumeQuota, and the pair of them is how the same operation came to be
// charged through one door and not the other. quota.Charge is the only one now,
// and it makes both of the checks this used to test — an empty caller and a
// zero cost — for every caller rather than only for pages.
//
// TestOneWayToCharge holds that.
