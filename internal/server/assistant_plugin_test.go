package server

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAssistantPluginOwnership(t *testing.T) {
	tokens := map[string]string{}
	for _, owner := range []string{"plugin_alice", "plugin_bob"} {
		if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
			t.Fatal(err)
		}
		_, key, err := auth.CreateToken(owner, "plugin", []string{"read", "api:agent", "api:inbox"}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		tokens[owner] = key
	}
	th := thread.Open("plugin_alice", thread.WebClient, "plugin-private")
	for _, name := range []string{"read", "thread"} {
		for _, owner := range []string{"plugin_alice", "plugin_bob"} {
			args := map[string]any{"id": th.ID}
			if name == "thread" {
				args = map[string]any{"thread": th.ID}
			}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
			r := httptest.NewRequest("POST", "https://mu.example/mcp/assistant", strings.NewReader(string(body)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+tokens[owner])
			if !scopedRequestAllowed(r) || !csrfExempt(r) {
				t.Fatal("plugin blocked by middleware")
			}
			w := httptest.NewRecorder()
			assistantPluginHandler(w, r)
			if w.Code != http.StatusOK {
				t.Fatal(w.Code, w.Body.String())
			}
			failed := strings.Contains(w.Body.String(), `"isError":true`)
			if failed != (owner == "plugin_bob") {
				t.Fatalf("%s %s: %s", name, owner, w.Body.String())
			}
		}
	}
}
