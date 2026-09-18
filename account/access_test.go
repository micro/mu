package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestAccessCompatibilityAndOAuth(t *testing.T) {
	const owner = "access_test_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Owner", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, values url.Values, csrf bool) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	r := request("GET", "/token?access=services", nil, false)
	w := httptest.NewRecorder()
	TokenHandler(w, r)
	if w.Code != 303 || w.Header().Get("Location") != "/account/tokens?access=services" {
		t.Fatalf("legacy HTML: %d %s", w.Code, w.Header())
	}
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	TokenHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("legacy JSON changed")
	}
	values := url.Values{"client_name": {"My app"}, "redirect_uris": {"https://example.com/callback"}}
	w = httptest.NewRecorder()
	TokenHandler(w, request("POST", "/account/tokens?create_client=1", values, false))
	if w.Code != 403 || len(auth.OAuthClientsFor(owner)) != 0 {
		t.Fatal("OAuth creation accepted missing CSRF")
	}
	values.Set("redirect_uris", "http://example.com/callback")
	w = httptest.NewRecorder()
	TokenHandler(w, request("POST", "/account/tokens?create_client=1", values, true))
	if w.Code != 400 {
		t.Fatal("accepted insecure remote redirect")
	}
	values.Set("redirect_uris", "https://example.com/callback")
	w = httptest.NewRecorder()
	TokenHandler(w, request("POST", "/account/tokens?create_client=1", values, true))
	clients := auth.OAuthClientsFor(owner)
	if w.Code != 303 || len(clients) != 1 {
		t.Fatalf("OAuth registration: %d %s", w.Code, w.Body.String())
	}
	foreign := auth.RegisterOAuthClient("another_account", "Foreign", []string{"https://example.com/callback"})
	w = httptest.NewRecorder()
	TokenHandler(w, request("POST", "/account/tokens?delete_client="+foreign.ClientID, url.Values{"_method": {"DELETE"}}, true))
	if w.Code != 403 || auth.GetOAuthClient(foreign.ClientID) == nil {
		t.Fatal("cross-account deletion allowed")
	}
	w = httptest.NewRecorder()
	TokenHandler(w, request("GET", "/account/tokens", nil, false))
	if !strings.Contains(w.Body.String(), "OAuth clients") || !strings.Contains(w.Body.String(), clients[0].ClientID) || strings.Contains(w.Body.String(), foreign.ClientID) || strings.Contains(w.Body.String(), clients[0].ClientSecret) {
		t.Fatal("OAuth listing leaks or omits clients")
	}
	for i := 0; i < 3; i++ {
		if _, _, err := auth.CreateToken(owner, "Script", []string{"read", "api:agent"}, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	first := auth.ListTokens(owner)
	for i := 0; i < 10; i++ {
		next := auth.ListTokens(owner)
		for j := range first {
			if first[j].ID != next[j].ID {
				t.Fatal("unstable order")
			}
			if j > 0 && next[j].Created.After(next[j-1].Created) {
				t.Fatal("not newest first")
			}
		}
	}
}
