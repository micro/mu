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
	for _, provider := range []string{"anthropic", "openai"} {
		if got := modelFor(provider, "claude-sonnet-4-6"); got != "claude-sonnet-4-6" {
			t.Errorf("%s: modelFor = %q, want the model untouched", provider, got)
		}
	}
}

// TestModelForAtlas — Atlas used to be reached only when the model named Atlas,
// so there was nothing to remap and this asserted the model came back
// untouched. Atlas is now also the last provider standing, which means it can
// be handed whatever DefaultModel returned. Same failure as OpenRouter's, same
// answer.
func TestModelForAtlas(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("ATLAS_API_KEY", "atlas-test")

	for _, c := range []struct{ name, model, want string }{
		{"a bare Claude id", "claude-sonnet-4-6", ModelDeepSeekPro},
		{"a hard-coded default", "claude-haiku-4-5-20251001", ModelDeepSeekPro},
		{"one of theirs", ModelDeepSeekFlash, ModelDeepSeekFlash},
		{"another of theirs", ModelQwenPlus, ModelQwenPlus},
	} {
		if got := modelFor("atlascloud", c.model); got != c.want {
			t.Errorf("%s: modelFor = %q, want %q", c.name, got, c.want)
		}
	}

	settings.Set("ATLAS_MODEL", ModelQwenPlus)
	if got := modelFor("atlascloud", "claude-sonnet-4-6"); got != ModelQwenPlus {
		t.Errorf("modelFor = %q, want the configured model", got)
	}
}

// TestAnAtlasKeyAloneIsAWorkingInstance. Configured() answers yes to a key, so
// an instance with only ATLAS_API_KEY turns the agent on — and every foreground
// request used to fail, because resolveProvider only claimed Atlas for models
// whose names said Atlas and DefaultModel returns a Claude id. Background work
// was fine, since BackgroundModel already returns an Atlas model when there is
// an Atlas key: the same instance summarised articles and could not answer a
// question.
func TestAnAtlasKeyAloneIsAWorkingInstance(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("ATLAS_API_KEY", "atlas-test")

	if !Configured() {
		t.Fatal("an Atlas key is not enough to be configured")
	}
	provider, key, _, err := resolveProvider(DefaultModel())
	if err != nil {
		t.Fatalf("configured, and yet: %v", err)
	}
	if provider != "atlascloud" || key != "atlas-test" {
		t.Fatalf("resolveProvider = %q with key %q", provider, key)
	}
	if got := modelFor(provider, DefaultModel()); !isAtlasModel(got) {
		t.Errorf("Atlas would be sent %q, which is not one of its models", got)
	}
}

// TestALocalServerStillWins — getAtlasAPIKey falls back to OPENAI_API_KEY, so
// the new fallback has to sit below the local branch. Somebody running Ollama
// with a key set should not be routed to a paid cloud on the strength of it.
func TestALocalServerStillWins(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENAI_API_KEY", "local-key")
	settings.Set("OPENAI_BASE_URL", "http://localhost:11434/v1")

	provider, _, baseURL, err := resolveProvider(DefaultModel())
	if err != nil {
		t.Fatal(err)
	}
	if provider != "openai" || baseURL != "http://localhost:11434/v1" {
		t.Errorf("resolveProvider = %q at %q, want the local server", provider, baseURL)
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
