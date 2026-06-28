package ai

import (
	"os"
	"testing"

	"mu/internal/settings"
)

// TestAskViaMicroLive checks the AI core routes through go-micro AND preserves
// conversation history. Gated on ATLAS_API_KEY (skips without it).
func TestAskViaMicroLive(t *testing.T) {
	key := os.Getenv("ATLAS_API_KEY")
	if key == "" {
		t.Skip("set ATLAS_API_KEY to run the live AI-core test")
	}
	settings.Set("ATLAS_API_KEY", key)

	out, err := Ask(&Prompt{
		System:   "You are concise.",
		Model:    ModelDeepSeekFlash,
		Caller:   "test",
		Question: "What is my dog's name and my favourite colour?",
		Context: History{
			{Prompt: "My favourite colour is teal and my dog is called Biscuit.", Answer: "Got it."},
		},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	t.Logf("reply: %s", out)
	low := out
	if !contains(low, "Biscuit") || !contains(low, "teal") {
		t.Fatalf("history not preserved through go-micro; reply=%q", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
