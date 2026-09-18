package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPBrowserAndProtocol(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRequest(method, "https://micro.mu/mcp", nil)
		w := httptest.NewRecorder()
		publicMCPHandler(w, r)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/developers?setup=mcp#mcp" {
			t.Fatalf("browser %s: %d %s", method, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "https://micro.mu/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	publicMCPHandler(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "protocolVersion") || w.Header().Get("Location") != "" {
		t.Fatalf("protocol: %d %s", w.Code, w.Body.String())
	}
}
