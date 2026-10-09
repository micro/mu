package server

import (
	"mu/account"
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHostAccountRedirect(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("MU_DOMAIN", "micro.test")
	for _, path := range []string{"/login?redirect=%2Faccount%2Ftopup", "/signup?redirect=%2Faccount", "/account?plan=starter", "/account/topup", "/account/tokens?access=services", "/oauth2/google?redirect=%2Faccount%2Ftopup", "/oauth2/callback?state=example&code=example", "/logout"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			r := httptest.NewRequest(method, "https://m3o.test"+path, nil)
			w := httptest.NewRecorder()
			if !redirectHostAccount(w, r) || w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://micro.test"+path {
				t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Header().Get("Location"))
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("authentication cookies must be set on the primary host")
			}
		}
	}
	for _, path := range []string{"/", "/tools", "/pricing", "/mcp", "/api/v1/web/search", "/oauth/authorize", "/oauth/token", "/.well-known/oauth-authorization-server"} {
		r := httptest.NewRequest("GET", "https://m3o.test"+path, nil)
		if redirectHostAccount(httptest.NewRecorder(), r) {
			t.Fatalf("redirected host protocol/public page %s", path)
		}
	}
	for _, tc := range []struct{ method, host, accept, auth string }{
		{"GET", "micro.test", "text/html", ""}, {"POST", "m3o.test", "text/html", ""}, {"GET", "m3o.test", "application/json", ""}, {"GET", "m3o.test", "text/html", "Bearer test"},
	} {
		r := httptest.NewRequest(tc.method, "https://"+tc.host+"/login", nil)
		r.Header.Set("Accept", tc.accept)
		r.Header.Set("Authorization", tc.auth)
		if redirectHostAccount(httptest.NewRecorder(), r) {
			t.Fatalf("unexpected redirect: %+v", tc)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost:8080/account/topup", nil)
	r.Header.Set("X-Forwarded-Host", "m3o.test")
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	if !redirectHostAccount(w, r) || w.Header().Get("Location") != "https://micro.test/account/topup" {
		t.Fatal("proxy host not recognized")
	}
	t.Setenv("MU_DOMAIN", "m3o.test")
	if redirectHostAccount(httptest.NewRecorder(), r) {
		t.Fatal("redirect loop")
	}
	t.Setenv("MU_DOMAIN", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("APP_URL", "")
	if redirectHostAccount(httptest.NewRecorder(), r) {
		t.Fatal("redirect without primary configuration")
	}
}

func TestHostSignInUsesPrimaryAccount(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("MU_DOMAIN", "micro.test")
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	t.Setenv("GOOGLE_REDIRECT_URI", "")
	const destination = "/account/topup"
	r := httptest.NewRequest("GET", "https://m3o.test/login?redirect="+url.QueryEscape(destination), nil)
	w := httptest.NewRecorder()
	if !redirectHostAccount(w, r) {
		t.Fatal("login stayed on payment host")
	}
	primaryLogin := w.Header().Get("Location")
	const id = "x402_primary_login_test"
	if err := auth.Create(&auth.Account{ID: id, Secret: "test-password", SecretSet: true}); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("POST", primaryLogin, strings.NewReader(url.Values{"id": {id}, "secret": {"test-password"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	account.Login(w, r)
	if w.Code != http.StatusFound || w.Header().Get("Location") != destination {
		t.Fatalf("password sign-in: %d %s", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session" {
			session = cookie
		}
	}
	if session == nil || session.Domain != "" || !session.Secure {
		t.Fatal("missing secure host-only session")
	}
	r = httptest.NewRequest("GET", "https://micro.test"+destination, nil)
	r.AddCookie(session)
	if _, acc := auth.TrySession(r); acc == nil || acc.ID != id {
		t.Fatal("primary account session not usable")
	}
	r = httptest.NewRequest("GET", "https://micro.test/oauth2/google?redirect="+url.QueryEscape(destination), nil)
	w = httptest.NewRecorder()
	account.GoogleLogin(w, r)
	google, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if google.Query().Get("redirect_uri") != "https://micro.test/oauth2/callback" {
		t.Fatal("Google callback left primary origin")
	}
	var state string
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "g_state" {
			state = cookie.Value
			if cookie.Domain != "" {
				t.Fatal("state cookie must be host-only")
			}
		}
	}
	if state == "" || state != google.Query().Get("state") {
		t.Fatal("Google state cookie missing or mismatched")
	}
}

func TestHostAccountPreservesLegacyCredentials(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("MU_DOMAIN", "micro.test")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range []string{"/account", "/account/tokens", "/account/topup"} {
			for _, accept := range []string{"", "text/html", "*/*"} {
				r := httptest.NewRequest(method, "https://m3o.test"+path, nil)
				r.Header.Set("X-Micro-Token", "legacy-credential")
				r.Header.Set("Accept", accept)
				w := httptest.NewRecorder()
				if redirectHostAccount(w, r) || w.Header().Get("Location") != "" {
					t.Fatalf("redirected credential-bearing %s %s (%q)", method, path, accept)
				}
				if r.Header.Get("X-Micro-Token") != "legacy-credential" {
					t.Fatal("changed request credential")
				}
			}
		}
	}
}
