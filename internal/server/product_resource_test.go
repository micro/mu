package server

import (
	"encoding/json"
	"mu/agent"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/work"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProductResourcesAreContentNegotiated(t *testing.T) {
	for _, tc := range []struct {
		method, path, content, accept string
		want                          bool
	}{
		{"POST", "/agent", "application/json", "", true},
		{"POST", "/agent/micro", "application/json", "", true},
		{"GET", "/inbox", "", "application/json", true},
		{"GET", "/work", "", "application/json", true},
		{"POST", "/work", "application/json", "", true},
		{"POST", "/inbox", "application/json", "", true},
		{"POST", "/agent/handoff", "application/json", "", false},
		{"POST", "/agent/run", "application/json", "", false},
		{"POST", "/agent/api/ask", "application/json", "", false},
		{"POST", "/agent/mcp", "application/json", "", false},
		{"POST", "/work", "application/x-www-form-urlencoded", "", false},
		{"GET", "/inbox", "", "text/html", false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Content-Type", tc.content)
		r.Header.Set("Accept", tc.accept)
		if productClientRequest(r) != tc.want {
			t.Errorf("%s %s (%s, %s)", tc.method, tc.path, tc.content, tc.accept)
		}
	}
}

func TestJSONResourceHandlers(t *testing.T) {
	const owner = "json_resource_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Test", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, token, err := auth.CreateToken(owner, "client", []string{"read", "write", "api:agent", "api:work", "api:inbox"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	old := api.Operations
	defer func() { api.Operations = old }()
	api.Operations = []api.Operation{{Name: "work_submit", Writes: true, Params: []api.ToolParam{{Name: "prompt", Type: "string", Required: true}}, Handle: func(account string, raw json.RawMessage) (any, error) {
		return map[string]string{"owner": account, "id": "test-work"}, nil
	}}}
	r := httptest.NewRequest("POST", "/work", strings.NewReader(`{"prompt":"test"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	work.Handler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "test-work") {
		t.Fatalf("work: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/inbox", nil)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	inbox.Handler(w, r)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("inbox: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("POST", "/agent", strings.NewReader(`{"prompt":""}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	agent.Handler(w, r)
	if w.Code != 400 {
		t.Fatalf("agent validation: %d %s", w.Code, w.Body.String())
	}
}
