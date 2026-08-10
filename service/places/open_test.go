package places

// A self-hosted instance does not meter, so it does not gate.
//
// The search and nearby handlers required a session with the comment "charged
// operation". Metering exists because micro.mu is run as a product — a price
// list, a balance, a card on file — so a charged call needs somebody to charge.
// An instance somebody runs for themselves has none of that.
//
// The person running it configured whatever keys it has and expects to use
// them. Being asked to sign in, on their own server, to spend their own Google
// quota, is the product's business model leaking into their deployment. Who may
// reach an exposed instance is a real question and a separate one: it is
// answered by who can sign up, not by pretending a lookup costs money where
// nothing is billed.

import (
	"net/http/httptest"
	"testing"

	"mu/billing"
)

func selfHosted(t *testing.T) {
	t.Helper()
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "")
	if billing.PaymentsEnabled() {
		t.Skip("this instance is configured to take payments")
	}
}

func TestASelfHostedLookupNeedsNoAccount(t *testing.T) {
	selfHosted(t)

	// With or without a key of their own: the operator configured it, and it
	// is their quota to spend.
	for _, key := range []string{"", "their-own-google-key"} {
		t.Setenv("GOOGLE_API_KEY", key)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/places/search", nil)
		id, ok := billableCaller(w, r, billing.OpPlacesSearch)
		if !ok {
			t.Errorf("GOOGLE_API_KEY=%q: an anonymous caller was refused on an "+
				"instance that charges nothing", key)
		}
		if id != "" {
			t.Errorf("resolved a caller %q where there is no session", id)
		}
	}
}

// Where the instance does charge, a charged call needs somebody to charge.
func TestWhereTheInstanceChargesALookupNeedsACaller(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_x")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_x")
	if !billing.PaymentsEnabled() {
		t.Skip("payments cannot be enabled in this environment")
	}
	if !billing.Metered(billing.OpPlacesSearch) {
		t.Skip("places search is not priced on this instance")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/places/search", nil)
	r.Header.Set("Accept", "application/json")

	if _, ok := billableCaller(w, r, billing.OpPlacesSearch); ok {
		t.Error("a charged lookup went through with nobody to charge")
	}
	if w.Code != 401 {
		t.Errorf("refused with %d, want 401", w.Code)
	}
}
