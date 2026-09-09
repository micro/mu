package agent

import (
	"context"
	gmai "go-micro.dev/v6/ai"
	"strings"
	"testing"
)

func TestFailedFetchIsNotSourceContent(t *testing.T) {
	for _, res := range []gmai.ToolResult{
		{Content: `{"error":"HTTP 403 403 Forbidden"}`},
		{Value: map[string]string{"error": "HTTP 403 403 Forbidden"}},
		{Value: map[string]any{"error": "HTTP 403 403 Forbidden"}},
	} {
		recorder := newNativeToolRecorder()
		handler := recorder.wrap(func(context.Context, gmai.ToolCall) gmai.ToolResult { return res })
		got := handler(context.Background(), gmai.ToolCall{Name: "web_Server_Fetch", ID: "blocked"})
		if !strings.Contains(got.Content, "This page was not read") {
			t.Fatal("model not told fetch failed")
		}
		for _, custom := range []bool{false, true} {
			answer := completeToolAnswerFor("HTTP 403 403 Forbidden", recorder.ragParts(), custom)
			if strings.Contains(answer, "403") || !strings.Contains(answer, "unavailable") {
				t.Fatalf("raw failure escaped: %s", answer)
			}
		}
	}
}

func TestHTTPDiscussionIsNotRawFailure(t *testing.T) {
	if isRawToolPayloadAnswer("HTTP 403 means the server refused access. You can read the other source.") {
		t.Fatal("explanation mistaken for raw failure")
	}
	if toolResultError(gmai.ToolResult{Content: `{"content":"An article about HTTP 403 errors"}`}) != "" {
		t.Fatal("article mistaken for failure")
	}
}
