package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWebFallbackFiltersMetadataInsideJSONEnvelope(t *testing.T) {
	text := `Web results for "Meta Muse":
Query intent: answer the user's original query "Meta Muse"; do not replace it with a broader meaning.
Confidence: high — synthesize only what the listed sources support.
Sources:
1. Meta introduces Muse — Muse is a personal AI agent that helps with everyday tasks. (https://example.com/muse)
2. How Muse works — The assistant connects to tools to carry out user requests. (https://example.com/how)`
	body, _ := json.Marshal(map[string]any{"text": text, "items": []map[string]string{{"title": "Meta introduces Muse", "url": "https://example.com/muse"}}})
	rag := []string{"### web_search\n" + string(body)}
	for _, custom := range []bool{false, true} {
		got := completeToolAnswerFor("Web results for Meta Muse: Query intent: answer the query", rag, custom)
		for _, internal := range []string{"Query intent:", "Confidence:", "Sources:", "Web results for", "synthesize only"} {
			if strings.Contains(got, internal) {
				t.Fatalf("internal guidance leaked: %s", got)
			}
		}
		if !strings.Contains(got, "personal AI agent") || !strings.Contains(got, "https://example.com/muse") {
			t.Fatalf("source evidence lost: %s", got)
		}
	}
}
