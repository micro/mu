package ai

import (
	"testing"

	"mu/internal/settings"
)

func clearProviders(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AI_PROVIDER", "ANTHROPIC_API_KEY", "ATLAS_API_KEY",
		"OPENROUTER_API_KEY", "OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_MODEL"} {
		t.Setenv(k, "")
		settings.Set(k, "")
	}
}

// One setting decides, and all three resolutions obey it.
//
// The order was hardcoded in three places, so an instance with $700 of Atlas
// credit and $34 of Anthropic sent its chat and its agent to Anthropic because
// a list in the source said Anthropic first.
func TestThePreferredProviderDecidesAllThree(t *testing.T) {
	clearProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AI_PROVIDER", "atlascloud")

	p, _, _, ok := PreferredProvider()
	if !ok || p != ProviderAtlasCloud {
		t.Fatalf("PreferredProvider = %q ok=%v, want atlascloud", p, ok)
	}
	// Chat.
	if got, _, _, err := resolveProvider(DefaultModel()); err != nil || got != ProviderAtlasCloud {
		t.Errorf("chat went to %q (err %v), want atlascloud", got, err)
	}
	// Background, which reached for Atlas on its own before and now does it
	// because it was told to.
	if got := BackgroundModel(); got != ModelDeepSeekFlash {
		t.Errorf("BackgroundModel = %q, want the preferred provider's cheap end", got)
	}
}

// The words `mu setup` writes are the words this accepts.
func TestSetupsOwnVocabularyWorks(t *testing.T) {
	clearProviders(t)
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	for _, spelling := range []string{"atlas", "atlascloud", "Atlas", " atlas "} {
		t.Setenv("AI_PROVIDER", spelling)
		if p, _, _, ok := PreferredProvider(); !ok || p != ProviderAtlasCloud {
			t.Errorf("%q gave %q ok=%v, want atlascloud", spelling, p, ok)
		}
	}
}

// A preference for a provider with no key is a typo, not an instruction.
func TestAPreferenceWithNoKeyIsIgnored(t *testing.T) {
	clearProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("AI_PROVIDER", "atlascloud") // no Atlas key

	if _, _, _, ok := PreferredProvider(); ok {
		t.Error("a provider with no key was returned as usable")
	}
	// And the instance still answers, on the provider it can reach.
	if got, _, _, err := resolveProvider(DefaultModel()); err != nil || got != ProviderAnthropic {
		t.Errorf("resolveProvider = %q (err %v), want anthropic", got, err)
	}
}

// A named model still names its provider: the more specific statement wins.
func TestANamedModelBeatsThePreference(t *testing.T) {
	clearProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")
	t.Setenv("AI_PROVIDER", "anthropic")

	if got, _, _, err := resolveProvider("deepseek-ai/deepseek-v4-pro-0813"); err != nil ||
		got != ProviderAtlasCloud {
		t.Errorf("a DeepSeek id went to %q, want atlascloud even with a preference "+
			"for anthropic", got)
	}
}

// Unset, nothing changes.
func TestWithNoPreferenceTheOldOrderStands(t *testing.T) {
	clearProviders(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	if _, _, _, ok := PreferredProvider(); ok {
		t.Error("a preference was found with none set")
	}
	if got, _, _, err := resolveProvider(DefaultModel()); err != nil || got != ProviderAnthropic {
		t.Errorf("resolveProvider = %q, want the built-in order (anthropic)", got)
	}
}
