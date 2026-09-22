package account

import (
	"io"
	"mu/internal/auth"
	"mu/internal/google"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type googleTestTransport func(*http.Request) (*http.Response, error)

func (f googleTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGoogleSignInDisconnectIsIndependent(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	const id = "google_disconnect_test"
	const email = "google-disconnect@example.test"
	if err := auth.Create(&auth.Account{ID: id, Email: email, EmailVerified: true, SecretSet: true, Secret: "chosen-password"}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(id)
	if err != nil {
		t.Fatal(err)
	}
	google.Store(id, email, "test-token", []string{grants["gmail"].scope})
	req := func(csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/account", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		values := url.Values{"disconnect_google_signin": {"1"}}
		if csrf {
			values.Set("_csrf", auth.CSRFToken(r))
		}
		r.Body = io.NopCloser(strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		Account(w, r)
		return w
	}
	if w := req(false); w.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", w.Code)
	}
	if w := req(true); w.Code != 303 {
		t.Fatalf("disconnect: %d %s", w.Code, w.Body.String())
	}
	acc, _ := auth.GetAccount(id)
	if !acc.GoogleSignInDisabled || !acc.EmailVerified || acc.Email != email || !google.HasScope(id, grants["gmail"].scope) {
		t.Fatal("disconnect changed identity or service grants")
	}
	old := oauthHTTP
	t.Cleanup(func() { oauthHTTP = old })
	oauthHTTP = &http.Client{Transport: googleTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"access_token":"test"}`
		if strings.Contains(r.URL.Path, "userinfo") {
			body = `{"email":"` + email + `","email_verified":true}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	r := httptest.NewRequest("GET", "/oauth2/google/callback?state=test&code=test", nil)
	r.AddCookie(&http.Cookie{Name: "g_state", Value: "test"})
	w := httptest.NewRecorder()
	GoogleCallback(w, r)
	if w.Code != 403 {
		t.Fatalf("disconnected Google identity signed in: %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			t.Fatal("session issued after disconnect")
		}
	}
	r = httptest.NewRequest("GET", "/oauth2/google/callback", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w = httptest.NewRecorder()
	linkGoogleToCurrentAccount(w, r, &googleUser{Email: email, EmailVerified: true})
	acc, _ = auth.GetAccount(id)
	if w.Code != 302 || acc.GoogleSignInDisabled {
		t.Fatal("explicit reconnect failed")
	}
	body := renderGoogleCard(r, acc, "")
	if strings.Index(body, "Google sign-in") > strings.Index(body, "<details") || !strings.Contains(body, "Disconnect sign-in") {
		t.Fatal("sign-in controls hidden")
	}
}

func TestGoogleDisconnectRequiresAlternativeLogin(t *testing.T) {
	const id = "google_only_login_test"
	if err := auth.Create(&auth.Account{ID: id, Email: "only@example.test", EmailVerified: true, Secret: "random-secret"}); err != nil {
		t.Fatal(err)
	}
	if auth.DisableGoogleSignIn(id) == nil {
		t.Fatal("removed sole sign-in method")
	}
	if err := auth.SavePasskey(&auth.Passkey{ID: "google-fallback-key", Account: id}); err != nil {
		t.Fatal(err)
	}
	if err := auth.DisableGoogleSignIn(id); err != nil {
		t.Fatal(err)
	}
	if auth.DeletePasskey("google-fallback-key", id) == nil {
		t.Fatal("deleted last usable sign-in method")
	}
	if err := auth.SetSecret(id, "chosen-password"); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeletePasskey("google-fallback-key", id); err != nil {
		t.Fatal(err)
	}
}
