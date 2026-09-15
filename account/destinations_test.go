package account

import (
	"encoding/json"
	"fmt"
	"mu/internal/quota"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBillingClientKeepsBoundedReceiptsAndAllowance(t *testing.T) {
	const owner = "billing-client"
	cookie := holder(t, owner, "Billing")
	for i := 0; i < 23; i++ {
		if err := AddCredits(owner, 1, fmt.Sprintf("test-%d", i), map[string]interface{}{"private_receipt": "not-for-the-view"}); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest("GET", "/account/billing", nil)
	r.AddCookie(cookie)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Account(w, r)
	var state struct {
		Balance       int `json:"balance"`
		DailyCredits  int `json:"daily_credits"`
		IncludedToday int `json:"included_today"`
		Transactions  []struct {
			Label  string `json:"label"`
			Amount string `json:"amount_label"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Balance != 23 || len(state.Transactions) != 20 || state.Transactions[0].Label != "Deposit" || state.Transactions[0].Amount != "+1" {
		t.Fatalf("billing view lost receipt information: %+v", state)
	}
	if state.DailyCredits != quota.DailyCredits() || state.IncludedToday != IncludedToday(owner) {
		t.Fatalf("daily allowance missing: %+v", state)
	}
	if strings.Contains(w.Body.String(), "private_receipt") || strings.Contains(w.Body.String(), "not-for-the-view") {
		t.Fatal("billing view leaked transaction metadata")
	}
}

func TestAccountDestinationsSeparateForms(t *testing.T) {
	cookie := holder(t, "account_split", "Account Split")
	for _, path := range []string{"/account", "/account/profile", "/account/billing"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		Account(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `id="root"`) {
			t.Fatalf("%s: client missing", path)
		}
		r.Header.Set("Accept", "application/json")
		w = httptest.NewRecorder()
		Account(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"account_split"`) {
			t.Fatalf("%s: own settings missing", path)
		}
		for _, secret := range []string{`"secret"`, `"credential"`, `"token"`} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("%s exposed %s", path, secret)
			}
		}
		guest := httptest.NewRecorder()
		Account(guest, httptest.NewRequest("GET", path, nil))
		if guest.Code != http.StatusSeeOther {
			t.Fatalf("%s accessible signed out", path)
		}
	}

	body := url.Values{"save_name": {"1"}, "display_name": {"Changed"}}
	r := httptest.NewRequest("POST", "/account/profile", strings.NewReader(body.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	Account(w, r)
	if w.Header().Get("Location") != "/account/profile?saved=name" {
		t.Fatal("profile save leaves profile")
	}
}

func TestPreviousAccountDestinationsRedirect(t *testing.T) {
	cookie := holder(t, "account_alias", "Account Alias")
	for _, tc := range []struct{ path, want string }{
		{"/account/usage", "/account/billing"},
		{"/account/connections?linked=google", "/account?linked=google#connections"},
		{"/account?saved=converted", "/account/billing?saved=converted"},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		Account(w, r)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tc.want {
			t.Fatalf("%s: got %d %q", tc.path, w.Code, w.Header().Get("Location"))
		}
	}
}
