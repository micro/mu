package ai

import (
	"strings"
	"testing"
)

// Nothing configured offers nothing. A menu of models that cannot run is worse
// than no menu: every option is a failure somebody has to discover.
func TestNoKeysOfferNoChoices(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}
	if got := Choices(); len(got) != 0 {
		t.Errorf("an unconfigured instance offers %d models", len(got))
	}
}

// A key brings its own provider's models and nobody else's.
func TestAKeyOffersItsOwnProvider(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	got := Choices()
	if len(got) == 0 {
		t.Fatal("a configured provider offers nothing")
	}
	for _, c := range got {
		if c.Provider != ProviderAnthropic {
			t.Errorf("%s (%s) is offered with no key for it", c.ID, c.Provider)
		}
	}
	// Two ends of the ladder, because "use the cheap one for this" is the
	// reason to choose at all.
	if len(got) < 2 {
		t.Errorf("only %d Claude models offered; the point is choosing between them", len(got))
	}
}

// Two keys, two providers, and the ids are distinct — a duplicate would be one
// option that silently shadows another.
func TestTwoKeysOfferBoth(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	seen := map[string]bool{}
	providers := map[string]bool{}
	for _, c := range Choices() {
		if seen[c.ID] {
			t.Errorf("%s is offered twice", c.ID)
		}
		seen[c.ID] = true
		providers[c.Provider] = true
		if strings.TrimSpace(c.Label) == "" {
			t.Errorf("%s has no label, so a menu would show an id", c.ID)
		}
	}
	if !providers[ProviderAnthropic] || !providers[ProviderAtlasCloud] {
		t.Errorf("two keys offered %v", providers)
	}
}

// The guard on what may be stored against an agent.
func TestOnlyOfferedModelsPass(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	// Empty is the instance default and is always allowed — it is what every
	// agent has until somebody chooses, and what they go back to when a
	// provider is removed.
	if !Offered("") {
		t.Error("the instance default was refused")
	}
	if !Offered(ModelDeepSeekFlash) {
		t.Error("a model this instance serves was refused")
	}
	// Case, because a stored id and a typed one need not match exactly.
	if !Offered(strings.ToUpper(ModelDeepSeekFlash)) {
		t.Error("the guard is case-sensitive, so a stored id can be refused on re-save")
	}
	// A Claude id with no Anthropic key is a run that fails at the model call.
	if Offered("claude-sonnet-5") {
		t.Error("a model no provider here serves was accepted")
	}
	if Offered("something-nobody-serves") {
		t.Error("an invented model was accepted")
	}
}

// What a person reads. An id is an address, not a name.
func TestLabels(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "AI_PROVIDER"} {
		t.Setenv(k, "")
	}
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	if got := LabelFor(""); got != "Instance default" {
		t.Errorf("the empty model reads as %q", got)
	}
	if got := LabelFor(ModelDeepSeekFlash); got == ModelDeepSeekFlash {
		t.Errorf("a known model shows its id rather than a name: %q", got)
	}
	// An operator who set AGENT_MODEL by hand sees what they set, not nothing.
	if got := LabelFor("some-model-they-configured"); got != "some-model-they-configured" {
		t.Errorf("an unknown model reads as %q instead of itself", got)
	}
}
