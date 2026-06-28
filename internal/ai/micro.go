package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// maxTok caps the response length (via go-micro's WithMaxTokens). Cheap
// background callers get a tighter cap to reduce latency and cost.
func generateViaMicro(model, systemPrompt string, messages []map[string]string, caller string, maxTok int) (string, error) {
	provider, apiKey, baseURL, err := resolveProvider(model)
	if err != nil {
		return "", err
	}

	switch caller {
	case "article-summary", "auto-tag-post", "auto-tag-note", "topic-generation", "topic-summary":
		maxTok = 512
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
	if maxTok > 0 {
		opts = append(opts, gmai.WithMaxTokens(maxTok))
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

// streamViaMicro streams a model response through go-micro, invoking onToken
// for each content chunk and returning the full text. If the provider does not
// support streaming, it falls back to a single Generate call and emits the
// whole reply at once — so every caller works regardless of provider.
func streamViaMicro(model, systemPrompt string, messages []map[string]string, caller string, maxTok int, onToken func(string)) (string, error) {
	provider, apiKey, baseURL, err := resolveProvider(model)
	if err != nil {
		return "", err
	}
	switch caller {
	case "article-summary", "auto-tag-post", "auto-tag-note", "topic-generation", "topic-summary":
		maxTok = 512
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
	if maxTok > 0 {
		opts = append(opts, gmai.WithMaxTokens(maxTok))
	}
	m := gmai.New(provider, opts...)

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
	req := &gmai.Request{SystemPrompt: systemPrompt, Messages: history, Prompt: question}

	stream, err := m.Stream(context.Background(), req)
	if err != nil {
		// Provider can't stream — fall back to a single Generate.
		if errors.Is(err, gmai.ErrStreamingUnsupported) {
			out, gerr := generateViaMicro(model, systemPrompt, messages, caller, maxTok)
			if gerr != nil {
				return "", gerr
			}
			if onToken != nil && out != "" {
				onToken(out)
			}
			return out, nil
		}
		return "", fmt.Errorf("%s: %w", provider, err)
	}
	defer stream.Close()

	app.Log("ai", "[LLM] streaming via go-micro %s/%s", provider, useModel)
	var sb strings.Builder
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return sb.String(), rerr
		}
		if resp.Reply != "" {
			sb.WriteString(resp.Reply)
			if onToken != nil {
				onToken(resp.Reply)
			}
		}
	}
	return sb.String(), nil
}
