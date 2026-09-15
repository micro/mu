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
