package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAskUsesPublicAPIAndContinuesThread(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/agent/ask" {
			t.Errorf("path %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["agent"] != "research" || req["prompt"] != "hello" {
			t.Errorf("args %v", req)
		}
		if calls == 2 && req["thread"] != "owned-thread" {
			t.Errorf("lost thread: %v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"text":"answer","thread":"owned-thread"}}`))
	}))
	defer srv.Close()
	a := asker{url: srv.URL + "/api/v1/agent/ask", agent: "research", http: srv.Client()}
	for i := 0; i < 2; i++ {
		got, err := a.ask("hello")
		if err != nil || got != "answer" {
			t.Fatalf("%q %v", got, err)
		}
	}
}
