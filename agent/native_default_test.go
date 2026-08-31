package agent

import (
	"strings"
	"testing"

	"mu/internal/ai"
	"mu/internal/settings"
)

func TestNativeLLM_OpenRouter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	noProviders(t)
	prevAtlas := settings.Get("ATLAS_API_KEY")
	prevOR := settings.Get("OPENROUTER_API_KEY")
	prevModel := settings.Get("OPENROUTER_MODEL")
	t.Cleanup(func() {
		settings.Set("ATLAS_API_KEY", prevAtlas)
		settings.Set("OPENROUTER_API_KEY", prevOR)
		settings.Set("OPENROUTER_MODEL", prevModel)
	})
	settings.Set("ATLAS_API_KEY", "")
	settings.Set("OPENROUTER_API_KEY", "")
	settings.Set("OPENROUTER_MODEL", "")

	if _, _, _, _, ok := nativeLLM(); ok {
		t.Fatal("no cloud key: nativeLLM should be off")
	}

	settings.Set("OPENROUTER_API_KEY", "sk_or_test")
	provider, key, model, _, ok := nativeLLM()
	if !ok || provider != "openrouter" || key != "sk_or_test" || model != ai.ModelOpenRouter {
		t.Fatalf("openrouter: provider=%q key=%q model=%q ok=%v", provider, key, model, ok)
	}

	settings.Set("ATLAS_API_KEY", "atlas_test")
	provider, key, model, _, ok = nativeLLM()
	if !ok || provider != "atlascloud" || key != "atlas_test" || model != ai.ModelDeepSeekPro {
		t.Fatalf("atlas still wins: provider=%q key=%q model=%q ok=%v", provider, key, model, ok)
	}
}

func TestNativeAgentInstanceNameIsUnique(t *testing.T) {
	first := nativeAgentInstanceName()
	second := nativeAgentInstanceName()

	if first == "" || second == "" {
		t.Fatal("native agent instance names should not be empty")
	}
	if first == second {
		t.Fatalf("native agent instance names should be unique, got %q twice", first)
	}
	if !strings.HasPrefix(first, "assistant-") || !strings.HasPrefix(second, "assistant-") {
		t.Fatalf("native agent instance names should keep assistant prefix, got %q and %q", first, second)
	}
}
