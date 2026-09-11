package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmai "go-micro.dev/v6/model"
)

func TestGeminiToolContinuation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			calls, executed := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"private reasoning","thought":true},{"functionCall":{"name":"events_List","args":{}},"thoughtSignature":"signed-part"}]}}]}`)
					return
				}
				contents := body["contents"].([]any)
				previous := contents[len(contents)-2].(map[string]any)
				parts := previous["parts"].([]any)
				if len(parts) != 2 || parts[1].(map[string]any)["thoughtSignature"] != "signed-part" {
					http.Error(w, `{"error":{"message":"Missing thoughtSignature"}}`, 400)
					return
				}
				if fail {
					http.Error(w, `{"error":{"message":"follow-up failed"}}`, 400)
					return
				}
				last := contents[len(contents)-1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
				result, ok := last["response"].(map[string]any)
				if !ok || !strings.Contains(fmt.Sprint(result), "Meeting at 9") {
					t.Errorf("missing tool result: %+v", last)
				}
				fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"Meeting at 9"}]}}]}`)
			}))
			defer srv.Close()
			model := gmai.New("gemini", gmai.WithBaseURL(srv.URL), gmai.WithAPIKey("test"), gmai.WithToolHandler(func(ctx context.Context, c gmai.ToolCall) gmai.ToolResult {
				executed++
				return gmai.ToolResult{Content: `{"events":"Meeting at 9","items":[]}`}
			}))
			rsp, err := model.Generate(context.Background(), &gmai.Request{Prompt: "What's on my calendar", Tools: []gmai.Tool{{Name: "events_List"}}})
			if executed != 1 || calls != 2 {
				t.Fatalf("calls=%d executions=%d", calls, executed)
			}
			if fail {
				if err == nil {
					t.Fatalf("follow-up failure swallowed: %+v", rsp)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rsp.Reply+rsp.Answer, "Meeting at 9") || strings.Contains(rsp.Reply+rsp.Answer, "private reasoning") {
				t.Fatalf("wrong final answer: %+v", rsp)
			}
		})
	}
}
