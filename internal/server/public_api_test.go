package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrimaryHostRoutesExposeOnlyOutcomeOperations(t *testing.T) {
	routesReady(t)
	r := httptest.NewRequest("GET", "https://micro.example/api/v1", nil)
	w := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(w, r)
	var catalogue struct {
		Operations []struct {
			Name string `json:"name"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &catalogue); err != nil {
		t.Fatal(err)
	}
	if len(catalogue.Operations) == 0 {
		t.Fatal("empty public catalogue")
	}
	groups := map[string]bool{}
	for _, op := range catalogue.Operations {
		g := strings.SplitN(op.Name, "_", 2)[0]
		groups[g] = true
		if g != "agent" && g != "work" && g != "inbox" {
			t.Fatalf("raw service escaped: %s", op.Name)
		}
	}
	if len(groups) != 3 {
		t.Fatalf("missing capability: %v", groups)
	}
	for _, path := range []string{"/api/v1/news/list", "/api/v1/tasks/create"} {
		w := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(w, httptest.NewRequest("POST", "https://micro.example"+path, strings.NewReader(`{}`)))
		if w.Code != 404 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	w = httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(w, httptest.NewRequest("POST", "https://micro.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
	if !strings.Contains(w.Body.String(), "agent_ask") || strings.Contains(w.Body.String(), "news_list") {
		t.Fatalf("wrong MCP route: %s", w.Body)
	}
}
