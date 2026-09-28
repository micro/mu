package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAskPromptOptions(t *testing.T) {
	for _, args := range [][]string{{"ask", "--prompt=whats the weather like"}, {"ask", "--prompt", "whats the weather like"}, {"ask", "whats the weather like"}} {
		t.Run(args[1], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["prompt"] != "whats the weather like" {
					t.Errorf("prompt: %q", body["prompt"])
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"text":"Sunny","thread":"test"}`))
			}))
			defer server.Close()
			t.Setenv("MU_URL", server.URL)
			t.Setenv("MU_TOKEN", "test-token")
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if Run(args) != 0 || calls != 1 {
				t.Fatalf("calls %d", calls)
			}
			if Run([]string{"ask", "--promt=typo"}) != 2 || calls != 1 {
				t.Fatal("unknown option sent as prompt")
			}
		})
	}
}
