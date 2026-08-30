package agent

import (
	"testing"

	"mu/agent/micro"
)

// An agent can name the model it answers with.
//
// One setting for the whole instance makes a single decision for jobs that are
// not alike. A lookup is one round and any competent model does it; a build is
// ten rounds of tool calls and rewards the model that holds together across
// them. With one dial you either pay frontier prices to answer "what's the
// weather", or you scrimp on the agent that writes programs.
//
// The resolution is the part worth pinning: naming a model has to pick the
// provider that serves it, not merely pass a string along. An Atlas id sent to
// Anthropic is a 400 on every question.
func TestAnAgentCanNameItsOwnModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AGENT_MODEL", "")

	// A DeepSeek id goes to Atlas, whatever else this box holds.
	provider, _, model, ok := nativeLLMFor("deepseek-ai/deepseek-v4-flash")
	if !ok || provider != "atlascloud" {
		t.Errorf("an Atlas model resolved to %q, want atlascloud", provider)
	}
	if model != "deepseek-ai/deepseek-v4-flash" {
		t.Errorf("the model asked for was not the model chosen: %q", model)
	}

	// A bare id is Anthropic's shape and goes there.
	provider, _, model, ok = nativeLLMFor("claude-sonnet-5")
	if !ok || provider != "anthropic" {
		t.Errorf("a bare model id resolved to %q, want anthropic", provider)
	}
	if model != "claude-sonnet-5" {
		t.Errorf("the model asked for was not the model chosen: %q", model)
	}

	// And naming none leaves the instance's own choice alone.
	if _, _, _, ok := nativeLLMFor(""); !ok {
		t.Error("an agent with no preference got no model at all")
	}
}

// A model this box cannot serve is ignored rather than fatal.
//
// The same rule AGENT_MODEL already had, for the same reason: a name whose
// provider has no key is a misconfiguration, not an instruction, and an agent
// that still answers beats one that fails closed over a typo. What makes it
// findable is the log line, not a broken agent.
func TestAModelWithNoKeyFallsBackRatherThanFailing(t *testing.T) {
	// Every variable the lookup reads, not the one it used to.
	//
	// This cleared ATLAS_API_KEY alone and passed for as long as that was the
	// only name Atlas answered to. getAtlasAPIKey reads three now —
	// ATLASCLOUD_API_KEY first — so on a machine with the real key exported the
	// test found one, routed to Atlas, and failed on an assertion about
	// something else entirely.
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	for _, k := range []string{"ATLASCLOUD_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY",
		"GEMINI_API_KEY", "OPENROUTER_API_KEY", "AGENT_MODEL", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}

	provider, _, model, ok := nativeLLMFor("deepseek-ai/deepseek-v4-flash")
	if !ok {
		t.Fatal("an unservable model left the agent with no model at all")
	}
	if provider == "atlascloud" {
		t.Error("routed to a provider whose key is not set")
	}
	if model == "deepseek-ai/deepseek-v4-flash" {
		t.Error("kept a model no configured provider can serve, which is a 400 " +
			"on every question rather than a wrong answer")
	}
}

// What an agent declares reaches the run.
//
// The field is worth nothing if the options built from it drop it on the way,
// and that is exactly the kind of gap nothing else notices — the agent answers,
// with the wrong model, and says nothing about it.
func TestAnAgentsModelReachesTheOptions(t *testing.T) {
	a := &micro.Agent{
		ID:           "example",
		SystemPrompt: "you are an example",
		Tools:        []string{"shell"},
		Model:        "deepseek-ai/deepseek-v4-flash",
	}
	if got := PlatformOpts(a).Model; got != a.Model {
		t.Errorf("PlatformOpts carried model %q, want %q", got, a.Model)
	}
	// And an agent with no preference passes none, rather than an empty string
	// that later reads as a choice.
	if got := PlatformOpts(&micro.Agent{ID: "plain"}).Model; got != "" {
		t.Errorf("an agent with no model preference produced %q", got)
	}
}
