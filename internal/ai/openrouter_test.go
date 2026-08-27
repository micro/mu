package ai

import (
	"testing"

	gmai "go-micro.dev/v6/ai"

	"mu/internal/settings"
)

var providerSettingKeys = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_MODEL",
	"ATLAS_API_KEY",
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
	"OPENROUTER_API_KEY",
	"OPENROUTER_MODEL",
}

// isolateProviderEnv points settings at a temp HOME and clears every AI
// provider key, so these tests do not read the operator's env or write
// their settings.json.
func isolateProviderEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	stored := map[string]string{}
	for _, k := range providerSettingKeys {
		t.Setenv(k, "")
		stored[k] = settings.Get(k)
		settings.Set(k, "")
	}
	t.Cleanup(func() {
		for k, v := range stored {
			settings.Set(k, v)
		}
	})
}

func TestOpenRouterProviderRegistered(t *testing.T) {
	m := gmai.New("openrouter", gmai.WithAPIKey("sk-or-test"))
	if m == nil {
		t.Fatal("openrouter provider was not registered")
	}
	if m.String() != "openai" {
		// The wrapper is go-micro's openai client pointed at OpenRouter.
		t.Fatalf("String() = %q, want openai (wrapped client)", m.String())
	}
	if got := m.Options().BaseURL; got != openRouterBaseURL {
		t.Fatalf("BaseURL = %q, want %q", got, openRouterBaseURL)
	}
}

func TestConfigured_OpenRouter(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")
	if !Configured() {
		t.Fatal("OPENROUTER_API_KEY should count as configured")
	}
}

func TestDefaultModel_OpenRouter(t *testing.T) {
	isolateProviderEnv(t)

	if got := DefaultModel(); got != "claude-sonnet-5" {
		t.Fatalf("no cloud key: DefaultModel = %q, want claude-sonnet-5", got)
	}

	settings.Set("OPENROUTER_API_KEY", "sk-or-test")
	if got := DefaultModel(); got != ModelOpenRouter {
		t.Fatalf("openrouter only: DefaultModel = %q, want %q", got, ModelOpenRouter)
	}

	settings.Set("OPENROUTER_MODEL", "anthropic/claude-sonnet-4")
	if got := DefaultModel(); got != "anthropic/claude-sonnet-4" {
		t.Fatalf("OPENROUTER_MODEL: DefaultModel = %q", got)
	}

	settings.Set("ANTHROPIC_API_KEY", "sk-ant-test")
	if got := DefaultModel(); got != "claude-sonnet-5" {
		t.Fatalf("anthropic wins interactive: DefaultModel = %q, want claude-sonnet-5", got)
	}
}

func TestBackgroundModel_OpenRouter(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")
	if got := BackgroundModel(); got != ModelOpenRouter {
		t.Fatalf("BackgroundModel = %q, want %q", got, ModelOpenRouter)
	}
	settings.Set("ATLAS_API_KEY", "atlas-test")
	if got := BackgroundModel(); got != ModelDeepSeekFlash {
		t.Fatalf("atlas still wins background: BackgroundModel = %q", got)
	}
}

func TestResolveProvider_OpenRouter(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")

	provider, key, base, err := resolveProvider(ModelOpenRouter)
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if provider != "openrouter" || key != "sk-or-test" || base != openRouterBaseURL {
		t.Fatalf("got provider=%q key=%q base=%q", provider, key, base)
	}

	// A bare Claude id still goes to OpenRouter when that is the only key —
	// DefaultModel has already remapped, but a leftover caller must not
	// fall through to "no provider".
	provider, _, _, err = resolveProvider("claude-sonnet-5")
	if err != nil || provider != "openrouter" {
		t.Fatalf("bare claude id with only OpenRouter: provider=%q err=%v", provider, err)
	}
}

func TestResolveProvider_OpenRouterDoesNotStealAtlas(t *testing.T) {
	isolateProviderEnv(t)
	settings.Set("ATLAS_API_KEY", "atlas-test")
	settings.Set("OPENROUTER_API_KEY", "sk-or-test")

	provider, key, _, err := resolveProvider(ModelDeepSeekFlash)
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if provider != "atlascloud" || key != "atlas-test" {
		t.Fatalf("atlas model should stay on Atlas, got provider=%q key=%q", provider, key)
	}
}

func TestProviderName_OpenRouter(t *testing.T) {
	if got := providerName("openai/gpt-4o-mini"); got != "openrouter" {
		t.Fatalf("providerName = %q, want openrouter", got)
	}
	if got := providerName("deepseek-ai/deepseek-v4-flash"); got != "atlas" {
		t.Fatalf("atlas slug should still be atlas, got %q", got)
	}
}
