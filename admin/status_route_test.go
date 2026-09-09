package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiagnosticsRedirectKeepsQuery(t *testing.T) {
	w := httptest.NewRecorder()
	DiagnosticsHandler(w, httptest.NewRequest("GET", "/admin/diagnostics?test=digest", nil))
	if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "/admin/status?test=digest" {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Location"))
	}
}
func TestStatusStillRequiresAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	StatusHandler(w, httptest.NewRequest("GET", "/admin/status", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status returned %d", w.Code)
	}
}
