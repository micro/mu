package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBootstrapEscapesUntrustedContent(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	Page(w, r, "<title>", map[string]any{"conversation": map[string]any{"text": "</script><script>alert(1)</script>"}})
	body := w.Body.String()
	if strings.Contains(body, "</script><script>alert(1)") || !strings.Contains(body, `\u003c/script\u003e`) {
		t.Fatal("bootstrap escaped incorrectly")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("personal state must not be cached")
	}
}
func TestPageDoesNotInterceptDataWritesOrSockets(t *testing.T) {
	for _, tc := range []struct{ method, header, value string }{{"POST", "", ""}, {"GET", "Accept", "application/json"}, {"GET", "Upgrade", "websocket"}} {
		r := httptest.NewRequest(tc.method, "/", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		w := httptest.NewRecorder()
		if Page(w, r, "Test") || w.Body.Len() != 0 {
			t.Fatalf("intercepted %v", tc)
		}
	}
}
func TestStaticLandingOnlyAtHome(t *testing.T) {
	for _, path := range []string{"/", "/notes"} {
		w := httptest.NewRecorder()
		Page(w, httptest.NewRequest("GET", path, nil), "Test")
		has := strings.Contains(w.Body.String(), `aria-label="Message Micro"`)
		if has != (path == "/") {
			t.Fatalf("landing at %s: %v", path, has)
		}
	}
}
