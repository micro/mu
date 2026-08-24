package ai

// Which provider this instance prefers, when it has keys for more than one.
//
// The order was hardcoded — Anthropic, then OpenRouter, then local, then Atlas
// — and three separate functions each had their own version of it. That is
// fine on an instance with one key, which is what `mu setup` asks for and what
// the README tells people to set. It is wrong the moment there are two, and it
// was wrong in a way nobody could see: an instance with $700 of Atlas credit
// and $34 of Anthropic sent its chat to Anthropic because the list said so.
//
// So the preference is a setting rather than a source order. One line, read in
// one place, and the three resolutions ask it rather than each carrying their
// own list.
//
// # What it does not override
//
// A named model. AGENT_MODEL and a model passed to Ask both name a provider by
// naming a model — deepseek-ai/… is Atlas's whatever this says — because the
// more specific statement wins. This decides where a request goes when nothing
// has said otherwise, which is almost all of them.

import (
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/settings"
)

// Provider names, as AI_PROVIDER accepts them. The spellings `mu setup` writes
// are accepted too: it stores "claude" and "atlas", which are the words on its
// own menu.
// ProviderAnthropic is in ai.go, which had it first.
const (
	ProviderAtlasCloud = "atlascloud"
	ProviderOpenRouter = "openrouter"
	ProviderLocal      = "openai"
)

// PreferredProvider is the provider this instance is configured to use, and the
// credential for it.
//
// Empty when nothing is set, or when what is set has no key — a preference for
// a provider you cannot reach is a misconfiguration rather than an instruction,
// so it is ignored, said once, and the ordinary order runs. An instance that
// still answers beats one that fails closed on a typo.
func PreferredProvider() (provider, key, baseURL string, ok bool) {
	want := normaliseProvider(settings.Get("AI_PROVIDER"))
	if want == "" {
		return "", "", "", false
	}
	switch want {
	case ProviderAnthropic:
		if k := settings.Get("ANTHROPIC_API_KEY"); k != "" {
			return ProviderAnthropic, k, "", true
		}
	case ProviderAtlasCloud:
		if k := getAtlasAPIKey(); k != "" {
			return ProviderAtlasCloud, k, "", true
		}
	case ProviderOpenRouter:
		if k := getOpenRouterAPIKey(); k != "" {
			return ProviderOpenRouter, k, openRouterBaseURL, true
		}
	case ProviderLocal:
		if u := settings.Get("OPENAI_BASE_URL"); u != "" {
			return ProviderLocal, settings.Get("OPENAI_API_KEY"), u, true
		}
		if u := detectOllama(); u != "" {
			return ProviderLocal, "", u, true
		}
	}
	unreachablePreference.Do(func() {
		app.Log("ai", "AI_PROVIDER is %q and there is no key or endpoint for it, "+
			"so it is being ignored", want)
	})
	return "", "", "", false
}

// unreachablePreference keeps that warning to once per process rather than
// once per request.
var unreachablePreference sync.Once

// normaliseProvider maps what somebody would write to what the code calls it.
//
// `mu setup` stores its own menu words — claude, atlas, ollama — so an operator
// who ran setup and then read this variable's documentation would otherwise
// have two vocabularies for one thing.
func normaliseProvider(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "anthropic", "claude":
		return ProviderAnthropic
	case "atlascloud", "atlas", "atlas_cloud", "atlas-cloud":
		return ProviderAtlasCloud
	case "openrouter", "open_router", "open-router":
		return ProviderOpenRouter
	case "openai", "local", "ollama":
		return ProviderLocal
	}
	return ""
}

// PreferredModel is the model to use for a tier on the preferred provider.
//
// Tiers rather than model names, because a tier means the same thing on every
// provider and a model name does not: "the one that reasons" is opus on
// Anthropic and deepseek-v4-pro on Atlas, and an instance that switches
// provider should not have to be told both.
func PreferredModel(provider string, background bool) string {
	switch provider {
	case ProviderAnthropic:
		if background {
			return backgroundAnthropic
		}
		return DefaultModel()
	case ProviderAtlasCloud:
		if background {
			return ModelDeepSeekFlash
		}
		return AtlasModel()
	case ProviderOpenRouter:
		return OpenRouterModel()
	}
	return ""
}

// backgroundAnthropic is the cheap end of Anthropic's ladder: summaries, tags,
// moderation and topics, which are high volume and barely care.
const backgroundAnthropic = "claude-haiku-4-5-20251001"
