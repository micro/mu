package shell

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
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
