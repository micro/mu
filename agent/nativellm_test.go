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
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AGENT_MODEL", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	provider, _, model, ok := nativeLLM()
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
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AGENT_MODEL", "deepseek-ai/deepseek-v4-pro-0813")

	provider, _, model, ok := nativeLLM()
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
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AGENT_MODEL", "")
	settings.Set("ANTHROPIC_API_KEY", "")
	t.Cleanup(func() { settings.Set("ANTHROPIC_API_KEY", "") })

	provider, _, _, ok := nativeLLM()
	if !ok {
		t.Fatal("no provider chosen with an Atlas key")
	}
	if provider != "atlascloud" {
		t.Errorf("provider = %q, want atlascloud", provider)
	}
}
