package ai

import (
	"testing"

	"mu/internal/settings"
)

// A model and a provider are chosen by separate rules, so a model can arrive
// somewhere that has never heard of it. OpenRouter answers a Claude id with a
// 400, and two ordinary configurations produce one: ANTHROPIC_MODEL left set
// from a previous provider — which DefaultModel returns before it considers
// OpenRouter at all — and any caller still holding a hard-coded default.
func TestModelForOpenRouter(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")

	for _, c := range []struct{ name, model, want string }{
		{"a bare Claude id", "claude-sonnet-4-6", ModelOpenRouter},
		{"a hard-coded default", "claude-haiku-4-5-20251001", ModelOpenRouter},
		{"an Atlas slug with no Atlas key", ModelDeepSeekFlash, ModelOpenRouter},
		{"a slug OpenRouter knows", "anthropic/claude-sonnet-4", "anthropic/claude-sonnet-4"},
	} {
		if got := modelFor("openrouter", c.model); got != c.want {
			t.Errorf("%s: modelFor = %q, want %q", c.name, got, c.want)
		}
	}

	// The configured override is what gets substituted, not the built-in.
	settings.Set("OPENROUTER_MODEL", "google/gemini-2.5-flash")
	if got := modelFor("openrouter", "claude-sonnet-4-6"); got != "google/gemini-2.5-flash" {
		t.Errorf("modelFor = %q, want the configured slug", got)
	}

	// Every other provider is left alone: Anthropic wants the Claude id, and a
	// local server has its own remap below this one.
	for _, provider := range []string{"anthropic", "atlascloud", "openai"} {
		if got := modelFor(provider, "claude-sonnet-4-6"); got != "claude-sonnet-4-6" {
			t.Errorf("%s: modelFor = %q, want the model untouched", provider, got)
		}
	}
}

// The case the PR's own test described — a leftover caller must not fall
// through to "no provider" — now resolves to a provider and a model that go
// together, rather than to a provider and a model that do not.
func TestOpenRouterGetsAModelItAccepts(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")
	settings.Set("ANTHROPIC_MODEL", "claude-sonnet-4-6")

	model := DefaultModel()
	if model != "claude-sonnet-4-6" {
		t.Fatalf("DefaultModel = %q — this test is about the case where it returns the leftover", model)
	}
	provider, _, _, err := resolveProvider(model)
	if err != nil || provider != "openrouter" {
		t.Fatalf("resolveProvider = %q, %v", provider, err)
	}
	if got := modelFor(provider, model); got != ModelOpenRouter {
		t.Errorf("modelFor = %q, want %q — OpenRouter cannot answer a Claude id", got, ModelOpenRouter)
	}
}
