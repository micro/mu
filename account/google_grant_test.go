package account

import (
	"mu/internal/google"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGoogleDataConnectionsAreSeparateAndBoundToTheAccount(t *testing.T) {
	withGoogle(t)
	cookie := holder(t, "google_grants_reader", "Reader")
	for _, what := range []string{"gmail", "drive", "calendar", "contacts"} {
		r := httptest.NewRequest("GET", "https://micro.mu/oauth2/google/"+what, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		GoogleGrantConnect(w, r)
		dest, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusSeeOther || dest.Host != "accounts.google.com" || dest.Query().Get("scope") != "openid email "+grants[what].scope {
			t.Fatalf("wrong grant for %s", what)
		}
		bound := false
		for _, c := range w.Result().Cookies() {
			if c.Name == "g_owner" && c.Value == "google_grants_reader" && c.HttpOnly && c.Secure {
				bound = true
			}
		}
		if !bound {
			t.Fatal("grant is not bound to initiating account")
		}
	}
	r := httptest.NewRequest("GET", "https://micro.mu/oauth2/google", nil)
	w := httptest.NewRecorder()
	GoogleLogin(w, r)
	dest, _ := url.Parse(w.Header().Get("Location"))
	if strings.Contains(dest.Query().Get("scope"), "readonly") {
		t.Fatal("sign-in asks for data access")
	}
}
func TestGoogleCallbackCannotAttachGrantToAnotherAccount(t *testing.T) {
	withGoogle(t)
	r := httptest.NewRequest("GET", "https://micro.mu/oauth2/callback", nil)
	r.AddCookie(holder(t, "google_grant_other", "Other"))
	r.AddCookie(&http.Cookie{Name: "g_owner", Value: "first-account"})
	w := httptest.NewRecorder()
	finishGoogleGrant(w, r, "gmail", "unused-code")
	if w.Code != http.StatusForbidden || google.Connected("google_grant_other") {
		t.Fatal("grant crossed accounts")
	}
}
func TestGoogleDisconnectRequiresFormToken(t *testing.T) {
	r := httptest.NewRequest("POST", "https://micro.mu/oauth2/google/disconnect", nil)
	r.AddCookie(holder(t, "google_disconnect_reader", "Reader"))
	w := httptest.NewRecorder()
	GoogleGrantDisconnect(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disconnect without CSRF: %d", w.Code)
	}
}
