package agent

import (
	"strings"
	"testing"

	"mu/internal/ai"
	"mu/internal/settings"
)

// TestNativeEnabledDefault: the go-micro agent is the default; only an explicit
// falsey AGENT_NATIVE disables it.
func TestNativeEnabledDefault(t *testing.T) {
	defer settings.Set("AGENT_NATIVE", "")
	settings.Set("AGENT_NATIVE", "")
	if !nativeEnabled() {
		t.Fatal("native agent should be enabled by default")
	}
	for _, off := range []string{"off", "false", "0", "no", "OFF"} {
		settings.Set("AGENT_NATIVE", off)
		if nativeEnabled() {
			t.Fatalf("AGENT_NATIVE=%q should disable the native agent", off)
		}
	}
	settings.Set("AGENT_NATIVE", "on")
	if !nativeEnabled() {
		t.Fatal("AGENT_NATIVE=on should keep it enabled")
	}
}

func TestNativeLLM_OpenRouter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"ATLAS_API_KEY", "OPENROUTER_API_KEY", "OPENROUTER_MODEL", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL"} {
		t.Setenv(k, "")
	}
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

	if _, _, _, ok := nativeLLM(); ok {
		t.Fatal("no cloud key: nativeLLM should be off")
	}

	settings.Set("OPENROUTER_API_KEY", "sk-or-test")
	provider, key, model, ok := nativeLLM()
	if !ok || provider != "openrouter" || key != "sk-or-test" || model != ai.ModelOpenRouter {
		t.Fatalf("openrouter: provider=%q key=%q model=%q ok=%v", provider, key, model, ok)
	}

	settings.Set("ATLAS_API_KEY", "atlas-test")
	provider, key, model, ok = nativeLLM()
	if !ok || provider != "atlascloud" || key != "atlas-test" || model != ai.ModelDeepSeekPro {
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
