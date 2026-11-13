package model

import (
	"fmt"
	"mu/internal/settings"
	"strings"
)

// ResolveLLM resolves an explicitly selected provider using runtime credentials.
// It never falls back to a different provider when configuration is missing.
func ResolveLLM(selection LLM, background bool) (provider, key, id, baseURL string, err error) {
	provider = normaliseProvider(selection.Provider)
	if provider == "" {
		err = fmt.Errorf("unknown model provider %q", selection.Provider)
		return
	}
	switch provider {
	case ProviderAnthropic:
		key = settings.Get("ANTHROPIC_API_KEY")
	case ProviderAtlasCloud:
		key = getAtlasAPIKey()
	case ProviderGemini:
		key = getGeminiAPIKey()
	case ProviderOpenRouter:
		key, baseURL = getOpenRouterAPIKey(), openRouterBaseURL
	case ProviderLocal:
		key, baseURL = settings.Get("OPENAI_API_KEY"), settings.Get("OPENAI_BASE_URL")
		if baseURL == "" {
			baseURL = detectOllama()
		}
	}
	if (provider == ProviderLocal && baseURL == "") || (provider != ProviderLocal && key == "") {
		err = fmt.Errorf("selected provider %q is not configured", provider)
		return
	}
	id = strings.TrimSpace(selection.ID)
	if id == "" {
		id = PreferredModel(provider, background)
		if provider == ProviderAnthropic && !background {
			id = settings.Get("ANTHROPIC_MODEL")
			if id == "" {
				id = ModelClaudeSonnet
			}
		}
	}
	if id == "" {
		err = fmt.Errorf("selected provider %q has no model configured", provider)
	}
	return
}
