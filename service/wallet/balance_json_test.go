package wallet

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A caller that asks for JSON gets JSON.
//
// wallet_balance is a path-backed tool pointed at /wallet, and /wallet renders
// the wallet page. The JSON balance was behind a ?balance=1 flag the tool
// declared as an optional param, so an agent calling a tool named
// "wallet_balance" without guessing the flag got 20KB of rendered HTML back.
// The tool dispatcher always sets Accept: application/json, so that header is
// the reliable signal.
func TestWalletHandlerHonoursAcceptJSON(t *testing.T) {
	for _, tc := range []struct{ name, url, accept string }{
		{"accept header", "/wallet", "application/json"},
		{"legacy flag", "/wallet?balance=1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			Handler(rec, req)

			body := rec.Body.String()
			if strings.Contains(body, "<html") || strings.Contains(body, "<div") {
				t.Fatalf("got a rendered page, not data: %.120s", body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type is %q, want JSON", ct)
			}
		})
	}
}

// A browser still gets the page. The Accept check must not turn /wallet into an
// API for everyone.
func TestWalletHandlerStillRendersThePage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "application/json") {
		t.Errorf("a browser got JSON (Content-Type %q)", ct)
	}
}
