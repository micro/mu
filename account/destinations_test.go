package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAccountDestinationsSeparateForms(t *testing.T) {
	cookie := holder(t, "account_split", "Account Split")
	for _, tc := range []struct{ path, want, absent string }{
		{"/account", "name=\"new_secret\"", "name=\"language\""},
		{"/account/profile", "name=\"display_name\"", "name=\"language\""},
		{"/account/billing", "View usage", "name=\"display_name\""},
		{"/account", "id=\"connections\"", "name=\"display_name\""},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		Account(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.absent) {
			t.Fatalf("%s: incorrect settings content", tc.path)
		}
		guest := httptest.NewRecorder()
		Account(guest, httptest.NewRequest("GET", tc.path, nil))
		if guest.Code != http.StatusSeeOther {
			t.Fatalf("%s accessible signed out", tc.path)
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
