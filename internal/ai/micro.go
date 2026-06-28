package ai

import (
	"context"
	"fmt"
	"strings"

	gmai "go-micro.dev/v6/ai"
	_ "go-micro.dev/v6/ai/anthropic"
	_ "go-micro.dev/v6/ai/atlascloud"
	_ "go-micro.dev/v6/ai/openai"

	"mu/internal/app"
	"mu/internal/settings"
)

// resolveProvider picks the go-micro ai provider and credentials for a model,
// mirroring mu's existing routing: Atlas Cloud for its models, then Anthropic
// if a key is set, otherwise a local OpenAI-compatible server (Ollama).
func resolveProvider(model string) (provider, apiKey, baseURL string, err error) {
	if isAtlasModel(model) && getAtlasAPIKey() != "" {
		return "atlascloud", getAtlasAPIKey(), "", nil
	}
	if key := settings.Get("ANTHROPIC_API_KEY"); key != "" {
		return "anthropic", key, "", nil
	}
	localURL := settings.Get("OPENAI_BASE_URL")
	localKey := settings.Get("OPENAI_API_KEY")
	if localURL == "" {
		localURL = detectOllama()
	}
	if localURL != "" {
		if localKey == "" {
			localKey = "ollama"
		}
		return "openai", localKey, localURL, nil
	}
	return "", "", "", fmt.Errorf("no AI provider configured — set ANTHROPIC_API_KEY, ATLAS_API_KEY or OPENAI_BASE_URL (Ollama)")
}

// generateViaMicro routes an LLM request through go-micro's ai package — the
// framework is the spine for every model call. The system prompt is sent
// separately, prior turns become conversation history, and the final user
// message is the prompt.
//
// maxTok is accepted for parity with mu's request shape but not yet applied:
// go-micro's ai package has no per-request max-tokens option (tracked as a
// follow-up); providers use their defaults, which return full responses.
func generateViaMicro(model, systemPrompt string, messages []map[string]string, caller string, maxTok int) (string, error) {
	provider, apiKey, baseURL, err := resolveProvider(model)
	if err != nil {
		return "", err
	}

	useModel := model
	if provider == "openai" && strings.HasPrefix(model, "claude") {
		useModel = detectLocalModel(baseURL, apiKey)
		if useModel == "" {
			return "", fmt.Errorf("no local model available")
		}
	}

	opts := []gmai.Option{gmai.WithAPIKey(apiKey), gmai.WithModel(useModel)}
	if baseURL != "" {
		opts = append(opts, gmai.WithBaseURL(baseURL))
	}
	m := gmai.New(provider, opts...)

	// Split mu's flat message list: skip the system role (sent separately),
	// keep prior turns as history, and pull the final user turn as the prompt.
	var history []gmai.Message
	for _, msg := range messages {
		if msg["role"] == "system" {
			continue
		}
		history = append(history, gmai.Message{Role: msg["role"], Content: msg["content"]})
	}
	var question string
	if n := len(history); n > 0 && history[n-1].Role == "user" {
		if s, ok := history[n-1].Content.(string); ok {
			question = s
		}
		history = history[:n-1]
	}

	app.Log("ai", "[LLM] via go-micro %s/%s", provider, useModel)
	resp, err := m.Generate(context.Background(), &gmai.Request{
		SystemPrompt: systemPrompt,
		Messages:     history,
		Prompt:       question,
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", provider, err)
	}

	recordUsage(caller, useModel, resp.Usage.InputTokens, resp.Usage.OutputTokens, 0, 0)
	app.Log("ai", "[LLM] Usage [%s]: input=%d output=%d (go-micro %s)",
		caller, resp.Usage.InputTokens, resp.Usage.OutputTokens, provider)
	return resp.Reply, nil
}
