package home

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExpiredInstalledSessionOpensLogin(t *testing.T) {
	w := httptest.NewRecorder()
	Index(w, httptest.NewRequest("GET", "/?from=app", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("PWA entry: %d %s", w.Code, w.Header().Get("Location"))
	}
}
