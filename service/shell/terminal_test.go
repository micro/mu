package shell

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTerminalOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"https://micro.example", true}, {"https://other.example", false}, {"http://micro.example", false},
		{"", false}, {"null", false}, {"https://micro.example.evil", false}, {"https://micro.example:8443", false},
	} {
		r := httptest.NewRequest("GET", "https://micro.example/shell", nil)
		r.Header.Set("Origin", tc.origin)
		if terminalOrigin(r) != tc.want {
			t.Errorf("origin %q", tc.origin)
		}
	}
}

func TestTerminalRequiresBrowserSession(t *testing.T) {
	for _, token := range []string{"", "invalid"} {
		r := httptest.NewRequest("GET", "https://micro.example/shell", nil)
		r.Header.Set("Authorization", "Bearer ignored")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		w := httptest.NewRecorder()
		terminalHandler(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d", w.Code)
		}
	}
}

func TestTerminalCSRFIsBoundToSession(t *testing.T) {
	requests := make([]*http.Request, 2)
	for i, id := range []string{"terminal-test-a", "terminal-test-b"} {
		auth.SetAccountForTest(&auth.Account{ID: id, Approved: true})
		defer auth.RemoveAccountForTest(id)
		sess, err := auth.CreateSession(id)
		if err != nil {
			t.Fatal(err)
		}
		defer auth.Logout(sess.Token)
		r := httptest.NewRequest("GET", "https://micro.example/shell", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		requests[i] = r
	}
	csrf := auth.CSRFToken(requests[0])
	if !terminalCSRF(requests[0], csrf) || terminalCSRF(requests[1], csrf) || terminalCSRF(requests[0], "") || terminalCSRF(requests[0], "forged") {
		t.Fatal("CSRF not bound to the browser session")
	}
	auth.SetAccountForTest(&auth.Account{ID: "terminal-test-a", Banned: true, Approved: true})
	if terminalCSRF(requests[0], csrf) {
		t.Fatal("banned account retained terminal access")
	}
	auth.SetAccountForTest(&auth.Account{ID: "terminal-test-a"})
	if terminalCSRF(requests[0], csrf) {
		t.Fatal("unverified account can mint terminal credentials")
	}
	auth.SetAccountForTest(&auth.Account{ID: "terminal-test-a", Approved: true})
	cookie, _ := requests[0].Cookie("session")
	if err := auth.Logout(cookie.Value); err != nil {
		t.Fatal(err)
	}
	if terminalCSRF(requests[0], csrf) {
		t.Fatal("revoked session retained terminal access")
	}

}

func TestTerminalLeaseReleased(t *testing.T) {
	if !claimTerminal("lease-test") {
		t.Fatal("first session refused")
	}
	if claimTerminal("lease-test") {
		t.Fatal("second session admitted")
	}
	releaseTerminal("lease-test")
	if !claimTerminal("lease-test") {
		t.Fatal("disconnected session not released")
	}
	releaseTerminal("lease-test")
}

func TestTerminalCheckReportsSharedModeWithoutOpeningMachine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL_SHARED", "true")
	const owner = "terminal-check-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Logout(sess.Token)
	r := httptest.NewRequest("POST", "https://micro.example/shell?terminal=check", nil)
	r.Header.Set("Origin", "https://micro.example")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "shared machines") {
		t.Fatalf("missing actual refusal: %d %s", w.Code, w.Body.String())
	}
	if !claimTerminal(owner) {
		t.Fatal("preflight reserved a terminal")
	}
	releaseTerminal(owner)
}

func TestTerminalOriginBehindConfiguredTLSProxy(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.example")
	r := httptest.NewRequest("GET", "http://micro.example/shell", nil)
	r.Header.Set("Origin", "https://micro.example")
	if !terminalOrigin(r) {
		t.Fatal("configured HTTPS origin rejected behind TLS proxy")
	}
	for _, value := range []string{"https://evil.example", "http://micro.example", "https://micro.example:8443"} {
		r.Header.Set("Origin", value)
		if terminalOrigin(r) {
			t.Fatalf("accepted foreign origin %s", value)
		}
	}
	r.Host = "other.example"
	r.Header.Set("Origin", "https://micro.example")
	if terminalOrigin(r) {
		t.Fatal("configured origin must not authorize another host")
	}
}
