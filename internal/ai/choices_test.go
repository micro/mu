package ai

import (
	"strings"
	"testing"
)

// Nothing configured offers nothing. A menu of models that cannot run is worse
// than no menu: every option is a failure somebody has to discover.
func TestNoKeysOfferNoChoices(t *testing.T) {
	clearProviders(t)
	if got := Choices(); len(got) != 0 {
		t.Errorf("an unconfigured instance offers %d models", len(got))
	}
}

// A key brings its own provider's models and nobody else's.
func TestAKeyOffersItsOwnProvider(t *testing.T) {
	clearProviders(t)
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
	clearProviders(t)
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
	clearProviders(t)
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
	clearProviders(t)
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

// Gemini is offered when its key is set, and routed by its own shape.
func TestGeminiIsOfferedAndRouted(t *testing.T) {
	clearProviders(t)
	t.Setenv("GEMINI_API_KEY", "gem-test")

	got := Choices()
	if len(got) == 0 {
		t.Fatal("a Gemini key offers nothing")
	}
	for _, c := range got {
		if c.Provider != ProviderGemini {
			t.Errorf("%s (%s) offered with only a Gemini key set", c.ID, c.Provider)
		}
		if !GeminiHosted(c.ID) {
			t.Errorf("%s is offered as Gemini's but is not recognised as one", c.ID)
		}
	}
	if !Configured() {
		t.Error("a Gemini key alone does not count as configured")
	}
	if p, _, _, ok := PreferredProvider(); ok {
		t.Errorf("no AI_PROVIDER set, yet %q is preferred", p)
	}
	t.Setenv("AI_PROVIDER", "gemini")
	if p, _, _, ok := PreferredProvider(); !ok || p != ProviderGemini {
		t.Errorf("AI_PROVIDER=gemini gives %q ok=%v", p, ok)
	}
}

// A Gemini id is a bare name with no slash in it, which is Anthropic's shape.
// Without GeminiHosted a run asked for one is sent to Anthropic and answered
// with a 400 on every question.
func TestAGeminiIdIsNotMistakenForAnthropics(t *testing.T) {
	for _, id := range []string{"gemini-pro-latest", "gemini-flash-latest", "gemini-3.1-pro-preview", "GEMINI-2.5-FLASH"} {
		if !GeminiHosted(id) {
			t.Errorf("%s is not recognised as Gemini's", id)
		}
	}
	for _, id := range []string{"claude-sonnet-5", "deepseek-ai/deepseek-v4-pro", "gpt-4o"} {
		if GeminiHosted(id) {
			t.Errorf("%s was claimed by Gemini", id)
		}
	}
}

// The Atlas key answers to the provider's own name, and still to ours.
//
// ATLASCLOUD_API_KEY is what the live instance sets and what an operator has in
// front of them in Atlas's dashboard. ATLAS_API_KEY was ours; it keeps working
// so the installs already on it do not break on an upgrade.
func TestAtlasAnswersToBothNames(t *testing.T) {
	for _, name := range []string{"ATLASCLOUD_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY"} {
		clearProviders(t)
		t.Setenv(name, "atlas-test")
		if got := AtlasKey(); got != "atlas-test" {
			t.Errorf("%s set and the key reads %q", name, got)
		}
	}
}
