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
// mirroring mu's existing routing: Atlas Cloud for its models, OpenRouter
// for provider/model slugs, then Anthropic if a key is set, then OpenRouter
// as the remaining cloud option, otherwise a local OpenAI-compatible server.
func resolveProvider(model string) (provider, apiKey, baseURL string, err error) {
	if isAtlasModel(model) && getAtlasAPIKey() != "" {
		return "atlascloud", getAtlasAPIKey(), "", nil
	}
	if isOpenRouterModel(model) && getOpenRouterAPIKey() != "" {
		return "openrouter", getOpenRouterAPIKey(), openRouterBaseURL, nil
	}
	// What this instance was told to prefer, before the built-in order.
	//
	// After the two branches above and not before them: those are a model
	// naming its own provider, which is the more specific statement and wins.
	// See PreferredProvider.
	if p, k, base, ok := PreferredProvider(); ok {
		return p, k, base, nil
	}
	if key := settings.Get("ANTHROPIC_API_KEY"); key != "" {
		return "anthropic", key, "", nil
	}
	if key := getOpenRouterAPIKey(); key != "" {
		return "openrouter", key, openRouterBaseURL, nil
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
	// Atlas as the remaining option, whatever the model was.
	//
	// Above, Atlas is only claimed for models whose names say Atlas — so an
	// instance whose only credential is ATLAS_API_KEY fell through every branch
	// and got "no AI provider configured". Configured() says that instance is
	// set up, because a key is a key; the agent is gated on and every foreground
	// request fails. Background ones did not, because BackgroundModel already
	// returns an Atlas model when there is an Atlas key, so the same instance
	// summarised articles happily and could not answer a question.
	//
	// Last, so a local server still wins: getAtlasAPIKey falls back to
	// OPENAI_API_KEY, and somebody running Ollama with a key set should not be
	// routed to a paid cloud on the strength of it.
	if key := getAtlasAPIKey(); key != "" {
		return "atlascloud", key, "", nil
	}
	return "", "", "", fmt.Errorf("no AI provider configured — set ANTHROPIC_API_KEY, ATLAS_API_KEY, OPENROUTER_API_KEY or OPENAI_BASE_URL (Ollama)")
}

// modelFor is the model id the resolved provider will actually accept.
//
// The provider and the model are chosen by separate rules, so a model can
// arrive somewhere that has never heard of it. An OpenAI-compatible local
// server already had this handled below; OpenRouter needs the same, because two
// ordinary configurations send it a Claude id it answers with a 400:
// ANTHROPIC_MODEL left set from a previous provider, which DefaultModel returns
// before it considers OpenRouter at all, and any caller still holding a
// hard-coded default. Both mean the same thing — this is not a slug OpenRouter
// knows — and the answer to both is the configured slug.
func modelFor(provider, model string) string {
	// Atlas is reached either because the model named it, or as the last
	// remaining provider — and in the second case the model is whatever
	// DefaultModel returned, which with no Anthropic key is a Claude id Atlas
	// has never heard of. Same failure as OpenRouter's below, same answer.
	if provider == "atlascloud" {
		if isAtlasModel(model) {
			return model
		}
		return AtlasModel()
	}
	if provider != "openrouter" {
		return model
	}
	// isOpenRouterModel only asks whether it looks like provider/model, and an
	// Atlas slug looks the same. Atlas is claimed first wherever both could
	// match, so reaching here with one means there was no Atlas key and it is
	// not a slug OpenRouter has either.
	if !isOpenRouterModel(model) || isAtlasModel(model) {
		return OpenRouterModel()
	}
	return model
}

// generateViaMicro routes an LLM request through go-micro's ai package — the
// framework is the spine for every model call. The system prompt is sent
// separately, prior turns become conversation history, and the final user
// message is the prompt.
//
// maxTok caps the response length (via go-micro's WithMaxTokens). Cheap
// background callers get a tighter cap to reduce latency and cost.
func generateViaMicro(model, systemPrompt string, messages []map[string]string, caller string, maxTok int) (reply string, err error) {
	// Every exit from this function is a verdict on whether the model answers.
	defer func() { recordHealth(err) }()

	provider, apiKey, baseURL, err := resolveProvider(model)
	if err != nil {
		return "", err
	}

	switch caller {
	case "article-summary", "auto-tag-post", "auto-tag-note", "topic-generation", "topic-summary":
		maxTok = 512
	}

	useModel := modelFor(provider, model)
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
func streamViaMicro(model, systemPrompt string, messages []map[string]string, caller string, maxTok int, onToken func(string)) (reply string, err error) {
	defer func() { recordHealth(err) }()

	provider, apiKey, baseURL, err := resolveProvider(model)
	if err != nil {
		return "", err
	}
	switch caller {
	case "article-summary", "auto-tag-post", "auto-tag-note", "topic-generation", "topic-summary":
		maxTok = 512
	}

	useModel := modelFor(provider, model)
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
	var usage gmai.Usage
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
		// The final chunk carries token usage (no content).
		if resp.Usage.TotalTokens > 0 || resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
			usage = resp.Usage
		}
	}
	recordUsage(caller, useModel, usage.InputTokens, usage.OutputTokens, 0, 0)
	app.Log("ai", "[LLM] Usage [%s]: input=%d output=%d (go-micro %s stream)",
		caller, usage.InputTokens, usage.OutputTokens, provider)
	return sb.String(), nil
}
