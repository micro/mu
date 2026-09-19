package server

import (
	"mu/internal/api"
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
		if w.Code != http.StatusOK || w.Header().Get("Location") != "" {
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

func TestRuntimeCatalogueDoesNotSelectProductOperations(t *testing.T) {
	old := api.Operations
	defer func() { api.Operations = old }()
	api.Operations = []api.Operation{{Name: "agent_product_marker"}}
	t.Setenv("X402_HOST", "m3o.test")
	for _, host := range []string{"micro.mu", "m3o.test"} {
		for _, bearer := range []string{"", "invalid"} {
			r := httptest.NewRequest("GET", "https://"+host+"/api/v1", nil)
			if bearer != "" {
				r.Header.Set("Authorization", "Bearer "+bearer)
			}
			w := httptest.NewRecorder()
			publicRESTHandler(w, r)
			if strings.Contains(w.Body.String(), "agent_product_marker") {
				t.Fatal("runtime REST exposed product operations")
			}
			r = httptest.NewRequest("POST", "https://"+host+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			if bearer != "" {
				r.Header.Set("Authorization", "Bearer "+bearer)
			}
			w = httptest.NewRecorder()
			publicMCPHandler(w, r)
			if strings.Contains(w.Body.String(), "agent_product_marker") {
				t.Fatal("runtime MCP exposed product operations")
			}
		}
	}
}
