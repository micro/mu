package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostedCommandsUseHTTPResources(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		path, method string
	}{
		{[]string{"agent", "create", "researcher", "--prompt", "Research with sources", "--tools", "web,news"}, "/agents", "POST"},
		{[]string{"agent", "list"}, "/agents", "GET"},
		{[]string{"work", "submit", "--agent", "researcher", "--prompt", "Compare options"}, "/work", "POST"},
		{[]string{"work", "get", "--id", "12345678901234567890"}, "/work", "GET"},
		{[]string{"inbox", "read", "--id", "thread"}, "/inbox", "GET"},
	} {
		t.Run(tc.args[0]+tc.args[1], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != tc.path || r.Method != tc.method || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				if r.Method == "POST" {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if r.URL.RawQuery != "" {
						t.Error("private request in URL")
					}
					if tc.path == "/agents" {
						if services, ok := body["services"].([]any); !ok || len(services) != 2 {
							t.Error("missing explicit service scope")
						}
					}
				} else if tc.args[1] == "get" && r.URL.Query().Get("id") != "12345678901234567890" {
					t.Error("numeric ID changed")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"result"}`))
			}))
			defer server.Close()
			t.Setenv("MU_URL", server.URL)
			t.Setenv("MU_TOKEN", "test-token")
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if Run(tc.args) != 0 || calls != 1 {
				t.Fatalf("calls: %d", calls)
			}
		})
	}
}

func TestProductMutationDoesNotRetryOrFollowRedirect(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "/elsewhere")
		w.WriteHeader(307)
	}))
	defer server.Close()
	_, err := NewClient(&ResolvedConfig{URL: server.URL, Token: "test"}).productRequest(productCommands["work_submit"], map[string]any{"prompt": "private"})
	if err == nil || calls != 1 {
		t.Fatalf("error %v, calls %d", err, calls)
	}
}
