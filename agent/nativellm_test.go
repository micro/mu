package agent

import (
	"testing"

	"mu/internal/ai"
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

// An OpenAI-compatible endpoint: Ollama, vLLM, llama.cpp.
//
// Not Gemini. This test pointed at Gemini's OpenAI-compatible shim, and that
// example cannot work: the provider builds its request as base +
// "/v1/chat/completions", and Gemini serves ".../v1beta/openai/chat/completions"
// — there is no value of OPENAI_BASE_URL that reaches it. Gemini has its own
// provider here and AI_PROVIDER=gemini is the way to it. The test passed anyway,
// because it only checked the string was carried through and never that the URL
// it produced was one that resolves.
func TestNativeAgentUsesOpenAICompatibleEndpoint(t *testing.T) {
	clearProviders(t)

	t.Setenv("AI_PROVIDER", "ollama")
	// As detectOllama returns it and as docs/INSTALL.md says to set it: with the
	// version segment on the end.
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("OPENAI_API_KEY", "local")
	t.Setenv("OPENAI_MODEL", "llama3.2")

	provider, key, model, baseURL, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen for an OpenAI-compatible endpoint")
	}
	if provider != ai.ProviderLocal || key != "local" || model != "llama3.2" {
		t.Fatalf("provider=%q key=%q model=%q, want the local configuration", provider, key, model)
	}

	// The version segment is gone, because the provider appends its own. With it
	// the request went to /v1/v1/chat/completions — a 404 from the endpoint the
	// install guide told the operator to set.
	if baseURL != "http://localhost:11434" {
		t.Fatalf("baseURL = %q, want the root — the provider appends /v1/chat/completions", baseURL)
	}

	a, _, built := buildNativeAgent("", "hello", QueryOpts{})
	if !built {
		t.Fatal("native agent was not built for an OpenAI-compatible endpoint")
	}
	if got := a.Options().BaseURL; got != baseURL {
		t.Fatalf("agent BaseURL = %q, want %q", got, baseURL)
	}
}

// A base URL with no model is half a configuration, and is reported as one.
//
// It defaulted to "gpt-4o-mini" — a model no Ollama or vLLM has ever heard of —
// so an operator who set only the endpoint got a 404 naming a model they never
// mentioned. "Not configured" is the true answer and it names what to set.
func TestAnEndpointWithNoModelIsNotConfigured(t *testing.T) {
	clearProviders(t)

	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("OPENAI_API_KEY", "local")

	if _, _, model, _, ok := nativeLLM(); ok {
		t.Errorf("an endpoint with no model resolved to %q; nothing here knows "+
			"what that server runs", model)
	}
	if _, working := Status(); working {
		t.Error("the status line claims the agent works with no model named")
	}
}

// clearProviders takes every provider key out of both stores, so a test says
// what this box holds rather than inheriting whatever the last one set.
//
// Both stores: settings.Get reads its own file as well as the environment, and
// a test that only called t.Setenv left a key behind for the next one.
func clearProviders(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AI_PROVIDER", "ANTHROPIC_API_KEY", "ATLAS_API_KEY", "ATLASCLOUD_API_KEY",
		"GEMINI_API_KEY", "OPENROUTER_API_KEY",
		"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL", "AGENT_MODEL",
	} {
		previous := settings.Get(key)
		settings.Set(key, "")
		t.Cleanup(func() { settings.Set(key, previous) })
		t.Setenv(key, "")
	}
}

// The key for a local endpoint stays local.
//
// getAtlasAPIKey read OPENAI_API_KEY as a third name for Atlas Cloud's key, and
// AtlasKey() is what decides whether this box "has Atlas". So setting the
// documented pair — a base URL and a key — started posting that key to Atlas
// Cloud, and the local server was never reached, because nativeLLMFor checks
// the Atlas key before it gets to OPENAI_BASE_URL.
//
// The arrangement that hid it is naming AI_PROVIDER, which takes the
// preferred-provider path above the check. That is what the first test here
// does, which is why it passed while the ordinary configuration did not work.
func TestALocalEndpointKeyIsNotAnAtlasKey(t *testing.T) {
	clearProviders(t)

	// No AI_PROVIDER: just the endpoint and its key, as INSTALL.md describes.
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("OPENAI_API_KEY", "local-only")
	t.Setenv("OPENAI_MODEL", "llama3.2")

	if k := ai.AtlasKey(); k != "" {
		t.Errorf("AtlasKey() = %q with only a local endpoint configured — that key "+
			"is for localhost and this is what sends it to Atlas Cloud", k)
	}

	provider, key, model, baseURL, ok := nativeLLM()
	if !ok {
		t.Fatal("a base URL, a key and a model is a complete configuration and got nothing")
	}
	if provider != ai.ProviderLocal {
		t.Fatalf("provider = %q, want the local endpoint the operator configured", provider)
	}
	if key != "local-only" || model != "llama3.2" || baseURL != "http://localhost:11434" {
		t.Fatalf("key=%q model=%q baseURL=%q, want the configured local server", key, model, baseURL)
	}
}
