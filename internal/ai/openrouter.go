package ai

import (
	"strings"

	gmai "go-micro.dev/v6/ai"
	"go-micro.dev/v6/ai/openai"

	"mu/internal/settings"
)

// OpenRouter is an OpenAI-compatible aggregator. go-micro has no OpenRouter
// provider and its agent cannot set a BaseURL, so Mu registers one itself —
// the same pattern as Atlas/Anthropic, kept in this repo rather than pushed
// upstream because Mu needed it.
//
// go-micro's openai client appends /v1/chat/completions, so the host is
// https://openrouter.ai/api and not the /api/v1 base the OpenRouter docs
// show for the OpenAI SDK.
const openRouterBaseURL = "https://openrouter.ai/api"

// ModelOpenRouter is the default OpenRouter slug for interactive and
// background calls. Override with OPENROUTER_MODEL. It has to be a
// provider/model slug OpenRouter recognises — not a bare Claude id —
// and it has to support tool calling, because the native agent uses it.
const ModelOpenRouter = "openai/gpt-4o-mini"

func init() {
	gmai.Register("openrouter", func(opts ...gmai.Option) gmai.Model {
		opts = append([]gmai.Option{gmai.WithBaseURL(openRouterBaseURL)}, opts...)
		return openai.NewProvider(opts...)
	})
	gmai.RegisterStream("openrouter")
}

func getOpenRouterAPIKey() string {
	return settings.Get("OPENROUTER_API_KEY")
}

// OpenRouterKey is the configured OpenRouter API key, if any.
func OpenRouterKey() string {
	return getOpenRouterAPIKey()
}

// OpenRouterModel is the model slug sent to OpenRouter.
func OpenRouterModel() string {
	if m := settings.Get("OPENROUTER_MODEL"); m != "" {
		return m
	}
	return ModelOpenRouter
}

// isOpenRouterModel reports whether the model id looks like an OpenRouter
// slug (provider/model). Atlas slugs also contain a slash; those are
// claimed first when an Atlas key is set.
func isOpenRouterModel(model string) bool {
	return strings.Contains(model, "/")
}
