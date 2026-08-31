package agent

import (
	"testing"

	"mu/internal/settings"
)

// The agent runs on the same provider the rest of the product prefers.
//
// nativeLLM had no Anthropic branch: Atlas came first, so an instance with both
// keys set ran the tool-calling loop on DeepSeek while ANTHROPIC_API_KEY served
// chat, summaries and moderation. Nothing on screen said so, and the symptom
// was an agent that felt worse than the product around it.
func TestTheAgentPrefersAnthropicLikeEverythingElse(t *testing.T) {
	noProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk_test")
	t.Setenv("ATLAS_API_KEY", "atlas_test")

	provider, _, model, _, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen with two keys set")
	}
	if provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic — the agent is the one path that "+
			"ignored the Anthropic key", provider)
	}
	if model == "" {
		t.Error("no model chosen")
	}
}

// And an operator who wants to spend Atlas credit says so, by naming a model.
func TestNamingAModelPicksItsProvider(t *testing.T) {
	noProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk_test")
	t.Setenv("ATLAS_API_KEY", "atlas_test")
	t.Setenv("AGENT_MODEL", "deepseek-ai/deepseek-v4-pro-0813")

	provider, _, model, _, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen")
	}
	if provider != "atlascloud" {
		t.Errorf("provider = %q, want atlascloud for a deepseek id", provider)
	}
	if model != "deepseek-ai/deepseek-v4-pro-0813" {
		t.Errorf("model = %q, want the one that was asked for", model)
	}
}

// Atlas alone still works, which is the self-hosted case with free credit.
func TestAtlasAloneStillRunsTheAgent(t *testing.T) {
	onlyProvider(t, "ATLAS_API_KEY", "atlas_test")
	settings.Set("ANTHROPIC_API_KEY", "")
	t.Cleanup(func() { settings.Set("ANTHROPIC_API_KEY", "") })

	provider, _, _, _, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen with an Atlas key")
	}
	if provider != "atlascloud" {
		t.Errorf("provider = %q, want atlascloud", provider)
	}
}

// A named model only ever reaches a provider that serves it.
//
// An Atlas slug and an OpenRouter slug are both provider/model, so trying each
// provider in turn and falling through on a missing key sends a DeepSeek id
// wherever a key happens to exist. ai.modelFor has a paragraph about this; the
// first version of nativeLLM recreated it one layer up, and the worst case is
// the quiet one — a deepseek-ai/… id handed to Anthropic, which answers with a
// 400 on every question anybody asks.
func TestANamedModelNeverReachesAProviderThatCannotServeIt(t *testing.T) {
	// A DeepSeek id, no Atlas key, but both other providers available.
	noProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk_test")
	t.Setenv("OPENROUTER_API_KEY", "or_test")
	t.Setenv("AGENT_MODEL", "deepseek-ai/deepseek-v4-pro-0813")
	settings.Set("ATLAS_API_KEY", "")
	t.Cleanup(func() { settings.Set("ATLAS_API_KEY", "") })

	provider, _, model, _, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen at all")
	}
	if model == "deepseek-ai/deepseek-v4-pro-0813" && provider != "atlascloud" {
		t.Errorf("a DeepSeek id was sent to %q, which cannot serve it", provider)
	}
	// It falls back to the default choice rather than failing closed: an agent
	// that answers beats one taken down by a typo.
	if provider != "anthropic" {
		t.Errorf("provider = %q, want the default choice (anthropic) once the "+
			"named model was ignored", provider)
	}
}

func TestNativeAgentUsesOpenAICompatibleEndpoint(t *testing.T) {
	for _, key := range []string{
		"AI_PROVIDER", "ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENROUTER_API_KEY",
		"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL", "AGENT_MODEL",
	} {
		previous := settings.Get(key)
		settings.Set(key, "")
		t.Cleanup(func() { settings.Set(key, previous) })
		t.Setenv(key, "")
	}

	t.Setenv("AI_PROVIDER", "ollama")
	t.Setenv("OPENAI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/")
	t.Setenv("OPENAI_API_KEY", "gemini-test-key")
	t.Setenv("OPENAI_MODEL", "gemini-2.5-flash")

	provider, key, model, baseURL, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen for an OpenAI-compatible endpoint")
	}
	if provider != "openai" || key != "gemini-test-key" || model != "gemini-2.5-flash" {
		t.Fatalf("provider=%q key=%q model=%q, want OpenAI-compatible configuration", provider, key, model)
	}
	if baseURL != "https://generativelanguage.googleapis.com/v1beta/openai/" {
		t.Fatalf("baseURL = %q, want configured endpoint", baseURL)
	}

	a, _, built := buildNativeAgent("", "hello", QueryOpts{})
	if !built {
		t.Fatal("native agent was not built for an OpenAI-compatible endpoint")
	}
	if got := a.Options().BaseURL; got != baseURL {
		t.Fatalf("agent BaseURL = %q, want %q", got, baseURL)
	}
}
