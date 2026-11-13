package model

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompositionReachesProvider(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		calls++
		if body.Model != "selected-model" {
			t.Errorf("model %q", body.Model)
		}
		found := false
		for _, m := range body.Messages {
			if strings.Contains(m.Content, "Composition instruction") {
				found = true
			}
		}
		if !found {
			t.Error("composition prompt did not reach provider")
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}]}`)
		}
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("AI_PROVIDER", "invalid-default")
	m := Default()
	m.LLM = LLM{Provider: "openai", ID: "selected-model"}
	m.Instructions = "Composition instruction"
	p := &Prompt{Definition: &m, Question: "Hello", Caller: "composition-test"}
	if out, err := Ask(p); err != nil || out != "answer" {
		t.Fatalf("Ask: %q %v", out, err)
	}
	if out, err := AskStream(p, func(string) {}); err != nil || out != "answer" {
		t.Fatalf("AskStream: %q %v", out, err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
	m.LLM.Provider = "missing"
	if _, err := Ask(p); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if calls != 2 {
		t.Fatal("invalid provider made a request")
	}
}
