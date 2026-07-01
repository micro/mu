package agent

import (
	"testing"

	gmai "go-micro.dev/v6/ai"
)

func TestNativeToolCallKeyDedupesEquivalentInputs(t *testing.T) {
	first := nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"category": "crypto", "limit": float64(10)}})
	second := nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"limit": float64(10), "category": "crypto"}})
	if first != second {
		t.Fatalf("expected equivalent native tool inputs to share a dedupe key: %q vs %q", first, second)
	}
	if first == nativeToolCallKey(gmai.ToolCall{Name: "markets_Get", Input: map[string]any{"category": "commodities", "limit": float64(10)}}) {
		t.Fatal("expected distinct native market inputs to keep distinct dedupe keys")
	}
}
