package ai

import "testing"

func TestAutomaticProcessingRequiresOptIn(t *testing.T) {
	t.Setenv("AI_BACKGROUND_ENABLED", "")
	for _, caller := range []string{"brief", "daily-digest", "moderate", "news-sentiment", "article-summary", "notes-generate", "opinion-generate", "social-judge", "topic-generation"} {
		if err := checkBackground(caller); err == nil {
			t.Fatalf("%s permitted", caller)
		}
	}
	if err := checkBackground("text"); err != nil {
		t.Fatal("explicit tool request blocked")
	}
	t.Setenv("AI_BACKGROUND_ENABLED", "true")
	if err := checkBackground("brief"); err != nil {
		t.Fatal(err)
	}
}
func TestProviderPreferenceBeatsStaleAnthropicModel(t *testing.T) {
	t.Setenv("AI_PROVIDER", "atlas")
	t.Setenv("ATLASCLOUD_API_KEY", "test")
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("ANTHROPIC_MODEL", ModelClaudeSonnet)
	if got := DefaultModel(); got == ModelClaudeSonnet {
		t.Fatal("stale Anthropic setting overrides selected provider")
	}
}

func TestUnavailableNamedModelNeverUsesStoredAnthropicKey(t *testing.T) {
	clearProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "test")
	for _, model := range []string{ModelDeepSeekFlash, ModelGeminiFlash, "google/gemini-2.5-flash"} {
		if p, _, _, err := resolveProvider(model); err == nil || p != "" {
			t.Fatalf("%s fell through to %s: %v", model, p, err)
		}
	}
}
