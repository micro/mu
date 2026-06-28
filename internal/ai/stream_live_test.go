package ai

import (
	"os"
	"strings"
	"testing"

	"mu/internal/settings"
)

// TestAskStreamViaMicroLive verifies AskStream streams through go-micro and
// preserves history. Gated on ATLAS_API_KEY.
func TestAskStreamViaMicroLive(t *testing.T) {
	key := os.Getenv("ATLAS_API_KEY")
	if key == "" {
		t.Skip("set ATLAS_API_KEY to run")
	}
	settings.Set("ATLAS_API_KEY", key)

	var chunks int
	var sb strings.Builder
	full, err := AskStream(&Prompt{
		System:   "You are concise.",
		Model:    ModelDeepSeekFlash,
		Caller:   "test",
		Question: "What is my dog's name?",
		Context:  History{{Prompt: "My dog is called Biscuit.", Answer: "Noted."}},
	}, func(tok string) {
		chunks++
		sb.WriteString(tok)
	})
	if err != nil {
		t.Fatalf("AskStream: %v", err)
	}
	t.Logf("chunks=%d full=%q", chunks, full)
	if !strings.Contains(full, "Biscuit") {
		t.Fatalf("history not preserved in stream: %q", full)
	}
	if sb.String() != full {
		t.Fatalf("streamed tokens (%q) != full reply (%q)", sb.String(), full)
	}
}
