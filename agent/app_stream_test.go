package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnswerDeltasAreOptIn(t *testing.T) {
	noProviders(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Stream bool }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
			w.(http.Flusher).Flush()
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"Hello world"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	t.Setenv("OPENAI_BASE_URL", srv.URL)
	t.Setenv("OPENAI_MODEL", "test-model")
	for _, tc := range []struct {
		name, prompt   string
		stream, deltas bool
	}{
		{"existing client", "Greet the reader", false, false},
		{"assistant app", "Greet the reader", true, true},
		{"news guard", "Explain today's latest AI news", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			streamNativeSSE(w, "", tc.prompt, QueryOpts{Public: true, System: "Answer briefly."}, &Flow{ID: "stream-test"}, "", tc.stream)
			body := w.Body.String()
			if strings.Contains(body, `"type":"stream_token"`) != tc.deltas {
				t.Fatalf("unexpected delta behaviour: %s", body)
			}
			if !strings.Contains(body, `"text":"Hello world"`) || !strings.Contains(body, `"type":"done"`) || strings.Contains(body, `"type":"error"`) {
				t.Fatalf("missing final answer: %s", body)
			}
		})
	}
}

func TestCalendarFallbackKeepsReadableEvents(t *testing.T) {
	payload := `{"items":[],"external":[],"total":1,"events":"- Fri 11 Sep 09:00 BST — Planning meeting (id: event-1)"}`
	answer := completeToolAnswerFor("Let me check your calendar.", []string{"### events\n" + formatToolResult("events_list", payload, nil)}, false)
	if !strings.Contains(answer, "Planning meeting") || strings.Contains(answer, "data is unavailable") {
		t.Fatalf("discarded calendar result: %s", answer)
	}
	empty := completeToolAnswerFor("Let me check your calendar.", []string{"### events\n" + formatToolResult("events_list", `{"items":[],"events":"No upcoming events."}`, nil)}, false)
	if !strings.Contains(empty, "No upcoming events") {
		t.Fatalf("empty calendar treated as failure: %s", empty)
	}
}
