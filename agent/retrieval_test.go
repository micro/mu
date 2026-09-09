package agent

import (
	"context"
	gmai "go-micro.dev/v6/ai"
	"mu/internal/thread"
	"testing"
	"time"
)

func TestRetrievedContextPreservesRoles(t *testing.T) {
	turns := []QueryMessage{{Role: "user", Text: "first"}, {Role: "assistant", Text: "answer"}}
	m := memoryWithRetrieval("", turns, "Retrieved context: evidence")
	if len(m.msgs) != 3 || m.msgs[0].Role != "user" || m.msgs[1].Role != "assistant" || m.msgs[1].Content != "answer" || m.msgs[2].Content != "Retrieved context: evidence" {
		t.Fatalf("roles: %+v", m.msgs)
	}
	if len(turns) != 2 || turns[0].Text != "first" {
		t.Fatal("mutated stored history")
	}
}

func TestEvidenceDoesNotRecordActionsFailuresOrGuestResults(t *testing.T) {
	th := thread.Open("retrieval-test", "web", "evidence-test")
	defer thread.Forget("retrieval-test")
	for _, tc := range []struct {
		name   string
		public bool
		result gmai.ToolResult
	}{
		{"mail_send", false, gmai.ToolResult{Value: "sent"}},
		{"shell_exec", false, gmai.ToolResult{Value: "secret"}},
		{"web_fetch", true, gmai.ToolResult{Value: "guest"}},
		{"web_fetch", false, gmai.ToolResult{Value: map[string]any{"error": "forbidden"}}},
	} {
		h := retainEvidence("retrieval-test", QueryOpts{Thread: th.ID, Public: tc.public})(func(context.Context, gmai.ToolCall) gmai.ToolResult { return tc.result })
		h(context.Background(), gmai.ToolCall{Name: tc.name, Input: map[string]any{}})
	}
	if len(thread.EvidenceFor("retrieval-test", th.ID, []string{"web", "mail", "shell"}, time.Now())) != 0 {
		t.Fatal("retained action, failure or guest output")
	}
	h := retainEvidence("retrieval-test", QueryOpts{Thread: th.ID})(func(context.Context, gmai.ToolCall) gmai.ToolResult { return gmai.ToolResult{Value: "source text"} })
	h(context.Background(), gmai.ToolCall{Name: "web_fetch", Input: map[string]any{"url": "https://example.com"}})
	if len(thread.EvidenceFor("retrieval-test", th.ID, []string{"web"}, time.Now())) != 1 {
		t.Fatal("lost source result")
	}
}

func TestFollowupRetrievalKeepsTopic(t *testing.T) {
	h := []QueryMessage{{Role: "user", Text: "what is Muse AI"}, {Role: "assistant", Text: "links"}}
	if retrievalQuery("read those links and summarise", h) != h[0].Text {
		t.Fatal("lost antecedent")
	}
	if retrievalQuery("hello", h) != "hello" {
		t.Fatal("borrowed unrelated previous topic")
	}
}
