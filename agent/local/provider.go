//go:build !android

package local

import (
	"fmt"
	gmai "go-micro.dev/v6/model"
	"go-micro.dev/v6/model/anthropic"
	"go-micro.dev/v6/model/atlascloud"
	"go-micro.dev/v6/model/openai"
	"mu/internal/ai"
	"mu/internal/settings"
)

// provider picks the model to talk to, in the order an operator would expect:
// the provider they configured explicitly, then the hosted default, then
// anything OpenAI-compatible they are running themselves.
func provider(handler gmai.ToolHandler) (gmai.Model, error) {
	opts := func(key, model string) []gmai.Option {
		return []gmai.Option{
			gmai.WithAPIKey(key),
			gmai.WithModel(model),
			gmai.WithToolHandler(handler),
		}
	}

	if key := settings.Get("ANTHROPIC_API_KEY"); key != "" {
		model := settings.Get("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-sonnet-5"
		}
		return anthropic.NewProvider(opts(key, model)...), nil
	}
	if key := settings.Get("ATLAS_API_KEY"); key != "" {
		model := settings.Get("ATLAS_MODEL")
		if model == "" {
			model = "deepseek-ai/deepseek-v4-pro"
		}
		return atlascloud.NewProvider(opts(key, model)...), nil
	}
	if key := settings.Get("OPENROUTER_API_KEY"); key != "" {
		model := settings.Get("OPENROUTER_MODEL")
		if model == "" {
			model = "anthropic/claude-sonnet-4.6"
		}
		// Without the version segment: the provider appends /v1/chat/completions
		// itself, so this said /api/v1 and asked for /api/v1/v1/chat/completions.
		// See ai.ProviderBaseURL.
		o := append(opts(key, model), gmai.WithBaseURL(ai.ProviderBaseURL("https://openrouter.ai/api/v1")))
		return openai.NewProvider(o...), nil
	}
	// A local server: Ollama and friends. The key is usually ignored but some
	// front ends insist on one being present.
	if base := settings.Get("OPENAI_BASE_URL"); base != "" {
		// The model the operator named. It defaulted to "gpt-4o-mini", which is
		// a model no Ollama or vLLM has ever heard of — so an operator who set
		// only the endpoint got a 404 naming a model they never mentioned,
		// instead of being told they had not said which model. See ai.LocalModel.
		model := ai.LocalModel()
		if model == "" {
			return nil, fmt.Errorf("set OPENAI_MODEL to the model this endpoint serves")
		}
		key := settings.Get("OPENAI_API_KEY")
		if key == "" {
			key = "local"
		}
		o := append(opts(key, model), gmai.WithBaseURL(ai.ProviderBaseURL(base)))
		return openai.NewProvider(o...), nil
	}

	return nil, fmt.Errorf("no model provider configured")
}

// Configured reports whether a model is available, so a caller can say what to
// set before doing any other work.
func Configured() bool {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ATLAS_API_KEY", "OPENROUTER_API_KEY", "OPENAI_BASE_URL"} {
		if settings.Get(k) != "" {
			return true
		}
	}
	return false
}

// Describe names the provider and model that will be used, for a startup line.
// A key can be set and still not answer, and that failure is much easier to
// place when the caller has already been told what it was going to try.
func Describe() string {
	switch {
	case settings.Get("ANTHROPIC_API_KEY") != "":
		return "anthropic"
	case settings.Get("ATLAS_API_KEY") != "":
		return "atlascloud"
	case settings.Get("OPENROUTER_API_KEY") != "":
		return "openrouter"
	case settings.Get("OPENAI_BASE_URL") != "":
		return "local (" + settings.Get("OPENAI_BASE_URL") + ")"
	}
	return "none"
}
