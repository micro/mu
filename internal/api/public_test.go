package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func publicFixture(t *testing.T) string {
	t.Helper()
	old := Operations
	t.Cleanup(func() { Operations = old })
	Operations = []Operation{{Name: "agent_ask", Writes: true, Params: []ToolParam{{Name: "prompt", Type: "string", Required: true}}, Handle: func(account string, raw json.RawMessage) (any, error) {
		return map[string]string{"account": account}, nil
	}},
		{Name: "inbox_list", Handle: func(account string, raw json.RawMessage) (any, error) {
			return map[string]string{"account": account}, nil
		}}}
	who := fmt.Sprintf("public_%x", sha256.Sum256([]byte(t.Name())))[:20]
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "test"}); err != nil {
		t.Fatal(err)
	}
	return who
}
func publicToken(t *testing.T, who string, perms ...string) string {
	t.Helper()
	_, secret, err := auth.CreateToken(who, "test", perms, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return secret
}
func publicRequest(t *testing.T, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	if path == "/mcp" {
		PublicMCPHandler(w, r)
	} else {
		PublicRESTHandler(w, r)
	}
	return w
}
func TestPublicTransportParity(t *testing.T) {
	who := publicFixture(t)
	key := publicToken(t, who, "read", "write", "api:agent")
	rest := publicRequest(t, "/api/v1/agent/ask", `{"prompt":"hello"}`, key)
	mcp := publicRequest(t, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agent_ask","arguments":{"prompt":"hello"}}}`, key)
	if rest.Code != 200 || mcp.Code != 200 {
		t.Fatalf("HTTP %d: %s; MCP %d: %s", rest.Code, rest.Body, mcp.Code, mcp.Body)
	}
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(mcp.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Result.Content) != 1 || strings.TrimSpace(out.Result.Content[0].Text) != strings.TrimSpace(rest.Body.String()) {
		t.Fatalf("responses differ: %s / %s", rest.Body, mcp.Body)
	}
}
func TestPublicScopeCannotWidenServiceKey(t *testing.T) {
	who := publicFixture(t)
	for _, tc := range []struct {
		name   string
		perms  []string
		status int
	}{
		{"full", []string{"read", "write"}, 200},
		{"agent", []string{"write", "api:agent"}, 200},
		{"read only", []string{"read", "api:agent"}, 403},
		{"inbox", []string{"write", "api:inbox"}, 403},
		{"service", []string{"read", "write", "service:news"}, 403},
		{"mixed", []string{"all", "service:news", "api:agent"}, 403},
		{"all with inbox", []string{"all", "api:inbox"}, 403},
	} {
		key := publicToken(t, who, tc.perms...)
		w := publicRequest(t, "/api/v1/agent/ask", `{"prompt":"hello"}`, key)
		if w.Code != tc.status {
			t.Errorf("%s: got %d: %s", tc.name, w.Code, w.Body)
		}
		mcp := publicRequest(t, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agent_ask","arguments":{"prompt":"hello"}}}`, key)
		if strings.Contains(mcp.Body.String(), `"isError":true`) != (tc.status != 200) {
			t.Errorf("%s MCP: %s", tc.name, mcp.Body)
		}
	}
}
func TestPublicCredentialWinsOverCookie(t *testing.T) {
	who := publicFixture(t)
	key := publicToken(t, who, "read")
	other := "public_other"
	_ = auth.Create(&auth.Account{ID: other, Secret: "test"})
	sess, err := auth.CreateSession(other)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{key, "invalid"} {
		r := httptest.NewRequest("POST", "/api/v1/inbox/list", strings.NewReader(`{}`))
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		PublicRESTHandler(w, r)
		if token == key {
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"account":"`+who+`"`) {
				t.Fatalf("wrong identity: %s", w.Body)
			}
		} else if w.Code != 401 {
			t.Fatalf("invalid key rescued by cookie: %d", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/inbox/list", strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	PublicRESTHandler(w, r)
	if w.Code != 403 {
		t.Fatalf("cookie without CSRF: %d", w.Code)
	}
}
func TestPublicRejectsRawToolsAndMalformedCalls(t *testing.T) {
	who := publicFixture(t)
	key := publicToken(t, who, "read", "write")
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/v1/news/list", `{}`, 404},
		{"/api/v1/agent/ask", `{"prompt":"hi","account":"victim"}`, 400},
		{"/api/v1/agent/ask", `{"prompt":"hi"} {}`, 400},
		{"/api/v1/agent/ask", `null`, 400},
	} {
		if w := publicRequest(t, tc.path, tc.body, key); w.Code != tc.status {
			t.Errorf("%s %s: %d", tc.path, tc.body, w.Code)
		}
	}
	w := publicRequest(t, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "")
	if strings.Contains(w.Body.String(), "probe_free") || !strings.Contains(w.Body.String(), "agent_ask") {
		t.Fatalf("wrong catalogue: %s", w.Body)
	}
	w = publicRequest(t, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"probe_free","arguments":{"query":"hi"}}}`, key)
	if !strings.Contains(w.Body.String(), "error") && !strings.Contains(w.Body.String(), "isError") {
		t.Fatalf("raw tool executed: %s", w.Body)
	}
}

func TestPublicInvalidPrimaryCredentialCannotHideScopedFallback(t *testing.T) {
	who := publicFixture(t)
	key := publicToken(t, who, "read", "write", "api:inbox")
	r := httptest.NewRequest("POST", "/api/v1/agent/ask", strings.NewReader(`{"prompt":"hello"}`))
	r.Header.Set("Authorization", "Bearer invalid")
	r.Header.Set(TokenHeader, key)
	w := httptest.NewRecorder()
	PublicRESTHandler(w, r)
	if w.Code != 401 {
		t.Fatalf("invalid primary credential fell back and lost scope: %d %s", w.Code, w.Body)
	}
}
