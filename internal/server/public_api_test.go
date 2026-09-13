package server

import (
	"context"
	"encoding/json"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestServicesTokenSelectsSameHostContract(t *testing.T) {
	routesReady(t)
	const owner = "same_host_services"
	if err := service.Register(service.Spec{Name: "routeprobe", Handler: &RouteProbe{}, Endpoints: map[string]service.Endpoint{"List": {}}}); err != nil {
		t.Fatal(err)
	}
	api.RegisterTool(api.Tool{Name: "routeprobe_list", Handle: func(map[string]any) (string, error) { return "service-ok", nil }})
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	defer auth.DeleteAccount(owner)
	_, key, err := auth.CreateToken(owner, "services", []string{"read", "write", "service:routeprobe"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body, want string }{
		{"/api/v1", "", `"service":"routeprobe"`},
		{"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, `routeprobe_list`},
		{"/api/v1/routeprobe/list", `{}`, `service-ok`},
		{"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"routeprobe_list","arguments":{}}}`, `service-ok`},
	} {
		r := httptest.NewRequest("POST", "https://micro.example"+tc.path, strings.NewReader(tc.body))
		if tc.body == "" {
			r.Method = "GET"
		}
		r.Header.Set("Authorization", "bearer "+key)
		r.Header.Set("Cookie", "session=unrelated")
		w := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(w, r)
		if !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), `agent_ask`) || strings.Contains(w.Body.String(), `news_list`) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "invalid")
	r.Header.Set("X-Micro-Token", key)
	if serviceAccess(r) {
		t.Fatal("invalid primary credential used fallback")
	}
	_, mixed, err := auth.CreateToken(owner, "mixed", []string{"service:routeprobe", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", mixed)
	if serviceAccess(r) {
		t.Fatal("mixed token selected services")
	}
	r.Header.Set("Authorization", key)
	r.Header.Set("Cookie", "session=unrelated")
	normalized := api.CredentialRequest(r)
	if normalized.Header.Get("Cookie") != "" {
		t.Fatal("cookie can override service credential")
	}
	_, acc, err := auth.RequireSession(normalized)
	if err != nil || acc.ID != owner {
		t.Fatal("service credential lost identity")
	}
}

type RouteProbe struct{}

func (*RouteProbe) List(context.Context, *struct{}, *struct{}) error { return nil }
