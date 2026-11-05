package auth

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNativeRedirectAllowlist(t *testing.T) {
	for _, uri := range []string{"mu.client.android://oauth/callback", "mu.client.android.debug://oauth/callback"} {
		if !RegisterableRedirect(uri) {
			t.Fatalf("native callback rejected: %s", uri)
		}
	}
	for _, uri := range []string{"mu.client.android://evil/callback", "mu.client.android://oauth/callback?next=evil", "mu.client.android://oauth/other", "other://oauth/callback"} {
		if RegisterableRedirect(uri) {
			t.Fatalf("unexpected app callback accepted: %s", uri)
		}
	}
}

func TestNativeExchangeRequiresPKCE(t *testing.T) {
	redirect := "mu.client.android://oauth/callback"
	for _, method := range []string{"", "plain"} {
		code := CreateAuthorizationCode("native-test", "unused-account", redirect, "", method, "read", "write", "api:agent")
		if _, err := ExchangeAuthorizationCode(code, "native-test", redirect, ""); err == nil || !strings.Contains(err.Error(), "S256") {
			t.Fatalf("native exchange without S256 accepted: %v", err)
		}
	}
	code := CreateAuthorizationCode("native-test", "unused-account", redirect, strings.Repeat("a", 43), "S256", "read", "write", "api:agent")
	if _, err := ExchangeAuthorizationCode(code, "native-test", redirect, "wrong-verifier"); err == nil {
		t.Fatal("native exchange accepted wrong verifier")
	}
}

func TestNativeAuthorizationProvidesExplicitAppReturn(t *testing.T) {
	for _, scheme := range []string{"mu.client.android", "mu.client.android.debug"} {
		w := httptest.NewRecorder()
		finishAuthorization(w, httptest.NewRequest("POST", "/oauth/authorize", nil), scheme+"://oauth/callback", "one-use-code", "state<&")
		if w.Code != 200 || w.Header().Get("Location") != "" {
			t.Fatalf("native flow still redirects: %d %v", w.Code, w.Header())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("authorization page must not be cached or leak a referrer")
		}
		body := html.UnescapeString(w.Body.String())
		start := strings.Index(body, "intent://")
		if start < 0 {
			t.Fatal("missing app return link")
		}
		end := strings.Index(body[start:], `"`)
		target := body[start : start+end]
		parts := strings.SplitN(target, "#Intent;", 2)
		if len(parts) != 2 || parts[1] != "scheme="+scheme+";package="+scheme+";end" {
			t.Fatalf("wrong Android intent: %s", target)
		}
		u, err := url.Parse(parts[0])
		if err != nil || u.Query().Get("code") != "one-use-code" || u.Query().Get("state") != "state<&" {
			t.Fatalf("callback data lost: %s", target)
		}
	}
	w := httptest.NewRecorder()
	finishAuthorization(w, httptest.NewRequest("POST", "/oauth/authorize", nil), "https://example.org/callback", "code", "state")
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "https://example.org/callback?") {
		t.Fatal("web OAuth redirect changed")
	}
}
