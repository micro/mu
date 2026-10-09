package server

import (
	"encoding/json"
	"mu/agent"
	"mu/agent/work"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/cli"
	host402 "mu/x402"
	"net/http"
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
		{"GET", "/agents", "", "application/json", true},
		{"POST", "/agents", "application/json", "", true},
		{"POST", "/agents", "application/x-www-form-urlencoded", "", false},
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

func TestScopedTokenCanVerifyIdentityOnly(t *testing.T) {
	owner := "scoped-verification"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	_, token, err := auth.CreateToken(owner, "cli", []string{"read", "write", "api:agent", "api:work"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{{"GET", "/session", true}, {"POST", "/session", false}, {"GET", "/account", false}, {"GET", "/session/other", false}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if scopedRequestAllowed(r) != tc.allowed {
			t.Fatalf("%s %s", tc.method, tc.path)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !scopedRequestAllowed(r) {
			http.Error(w, "Scoped token blocked", http.StatusForbidden)
			return
		}
		host402.SessionHandler(w, api.CredentialRequest(r))
	}))
	defer srv.Close()
	client := cli.NewClient(&cli.ResolvedConfig{URL: srv.URL, Token: token})
	if err := client.Verify(); err != nil {
		t.Fatalf("CLI verification of scoped token: %v", err)
	}
	client.Token = "invalid-token"
	if err := client.Verify(); err == nil {
		t.Fatal("invalid token passed CLI verification")
	}
	for _, path := range []string{"/work/123", "/inbox/abcdef012345abcdef012345"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		if !productClientRequest(r) || !scopedRequestAllowed(r) {
			t.Fatal("blocked resource path", path)
		}
	}
}
