package places

// Public data does not need an account.
//
// The search and nearby handlers required a session, with the comment "charged
// operation". Right reason, wrong condition. Without a Google key these answer
// from OpenStreetMap and Overpass, which cost nobody anything — so a
// self-hoster had to sign in before their own server would tell them where the
// nearest coffee is, on data that is free.
//
// What decides it is whether the provider costs money. A Google key means every
// lookup spends the operator's money and wants somebody named, even where no
// payment is configured — "we cannot bill you" is not a reason to let strangers
// spend it. No key means Overpass, and then an account is ceremony.

import (
	"net/http/httptest"
	"testing"

	"mu/service/wallet"
)

func TestWithNoGoogleKeyAndNoPaymentsAnyoneMayLookUpAPlace(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "")
	if wallet.PaymentsEnabled() {
		t.Skip("this instance is configured to take payments")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/places/search", nil)

	id, ok := billableCaller(w, r, wallet.OpPlacesSearch)
	if !ok {
		t.Fatal("an anonymous caller was refused free OpenStreetMap data on an " +
			"instance that cannot charge for it")
	}
	if id != "" {
		t.Errorf("resolved a caller %q where there is no session", id)
	}
}

// A key means the operator is paying per lookup. That wants a name on it
// whether or not this instance can bill.
func TestWithAGoogleKeyALookupNeedsACaller(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "test-key")
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_PUBLISHABLE_KEY", "")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/places/search", nil)
	r.Header.Set("Accept", "application/json")

	if _, ok := billableCaller(w, r, wallet.OpPlacesSearch); ok {
		t.Error("an anonymous caller can spend the operator's Google quota")
	}
	if w.Code != 401 {
		t.Errorf("refused with %d, want 401", w.Code)
	}
}
