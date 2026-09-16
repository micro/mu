package home

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicPagesUseLandingShell(t *testing.T) {
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/about", AboutHandler}, {"/contact", ContactHandler},
		{"/pricing", PricingHandler}, {"/privacy", PrivacyHandler}, {"/status", StatusHandler},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest("GET", tc.path, nil))
			body := w.Body.String()
			if w.Code != 200 || !strings.Contains(body, `<main>`) {
				t.Fatal("missing public page")
			}
			for _, marker := range []string{`id="nav"`, `id="footer"`} {
				if strings.Contains(body, marker) {
					t.Errorf("app shell leaked into public page: %s", marker)
				}
			}
			hasFooter := strings.Contains(body, `aria-label="Site information"`)
			if !hasFooter {
				t.Error("wrong footer visibility")
			}
			if !strings.Contains(w.Header().Get("Cache-Control"), "private") {
				t.Error("contact response must not be shared between accounts")
			}
		})
	}
}
