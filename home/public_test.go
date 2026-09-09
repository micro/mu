package home

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/app"
)

func TestPublicPagesUseLandingShell(t *testing.T) {
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/about", AboutHandler}, {"/contact", ContactHandler},
		{"/pricing", PricingHandler}, {"/privacy", PrivacyHandler}, {"/status", app.StatusHandler},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest("GET", tc.path, nil))
			body := w.Body.String()
			if w.Code != 200 || !strings.Contains(body, `<article class="public-page"><h1>`) {
				t.Fatal("missing public page")
			}
			for _, marker := range []string{`id="nav"`, `id="footer"`, `/mu.css?`} {
				if strings.Contains(body, marker) {
					t.Errorf("app shell leaked into public page: %s", marker)
				}
			}
			hasFooter := strings.Contains(body, `<div class="footer">`)
			if hasFooter != (tc.path != "/about") {
				t.Error("wrong footer visibility")
			}
			if !strings.Contains(w.Header().Get("Cache-Control"), "private") {
				t.Error("contact response must not be shared between accounts")
			}
		})
	}
}

func TestPricingExplainsWelcomeBalance(t *testing.T) {
	takesPayment(t)
	body := pricingPage(t)
	for _, want := range []string{"one-time welcome balance", "not a daily allowance", "Daily limits", "midnight UTC"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "thirty questions") {
		t.Error("hardcoded question estimate")
	}
}
