package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"mu/internal/app"
	"mu/internal/settings"
)

var (
	// Limit concurrent LLM requests to prevent memory bloat
	llmSemaphore = semaphore.NewWeighted(5)
	llmTimeout   = 120 * time.Second

	// Anthropic cache stats
	cacheStatsMu        sync.Mutex
	cacheHits           int
	cacheMisses         int
	cacheReadTokens     int
	cacheCreationTokens int
)

// getAtlasAPIKey reads Atlas Cloud's key.
//
// ATLASCLOUD_API_KEY is the name, because it is the provider's own and the one
// on the live instance. ATLAS_API_KEY was ours, shorter by four characters and
// different from what an operator copying a key out of Atlas's dashboard has in
// front of them — so it stays readable, for the installs already running on it,
// and is not what anything documents.
//
// OPENAI_API_KEY last, and only because Atlas speaks the OpenAI protocol: an
// instance pointed at a compatible endpoint had one key for both.
func getAtlasAPIKey() string {
	for _, k := range []string{"ATLASCLOUD_API_KEY", "ATLAS_API_KEY", "OPENAI_API_KEY"} {
		if v := settings.Get(k); v != "" {
			return v
		}
	}
	return ""
}

// getGeminiAPIKey reads Google's key.
//
// GEMINI_API_KEY is the name Google's own docs and console use. GOOGLE_API_KEY
// is deliberately not read: this instance already uses it for Maps, Places and
// Calendar, and a Maps key handed to the model API is a failure at the first
// call with nothing on screen to say the two are different things.
func getGeminiAPIKey() string {
	return settings.Get("GEMINI_API_KEY")
}

// GeminiModel is the model to send Gemini when the caller did not name one —
// the counterpart of AtlasModel and OpenRouterModel.
func GeminiModel() string {
	if m := settings.Get("GEMINI_MODEL"); m != "" {
		return m
	}
	return ModelGeminiPro
}

// Configured reports whether at least one AI provider is available — a key or
// endpoint set via env/settings, or a local Ollama detected. Used to gate the
// agent/chat and to decide whether a fresh instance still needs setup.
func Configured() bool {
	if settings.Get("ANTHROPIC_API_KEY") != "" {
		return true
	}
	if getAtlasAPIKey() != "" {
		return true
	}
	if getGeminiAPIKey() != "" {
		return true
	}
	if getOpenRouterAPIKey() != "" {
		return true
	}
	if settings.Get("OPENAI_BASE_URL") != "" {
		return true
	}
	return detectOllama() != ""
}

// Atlas Cloud model aliases — used to route requests to Atlas Cloud
// instead of Anthropic. Any model string starting with "deepseek" or
// "qwen" is routed to Atlas Cloud automatically.
const (
	ModelDeepSeekPro   = "deepseek-ai/deepseek-v4-pro"
	ModelDeepSeekFlash = "deepseek-ai/deepseek-v4-flash"
	ModelQwenPlus      = "qwen/qwen3.6-plus"

	// Gemini, by the aliases Google keeps pointed at the current generation
	// rather than by a pinned id.
	//
	// gemini-2.5-pro was the obvious constant to write and it is already
	// refused: "no longer available to new users. Please update your code."
	// A pinned model id is a decision with a shelf life measured in months,
	// and the place it is written down is not the place anybody revisits — the
	// same reason DefaultModel stopped naming claude-sonnet-4-6. GEMINI_MODEL
	// is how an operator pins one on purpose.
	ModelGeminiPro   = "gemini-pro-latest"
	ModelGeminiFlash = "gemini-flash-latest"

	// Anthropic, by name.
	//
	// Every other provider here had named constants and Anthropic did not, so
	// the menu could only offer "whatever DefaultModel returns" and "the cheap
	// one" — which is how an instance with an Anthropic key offered no way to
	// pick Opus. These are the three worth choosing between, and they are
	// pinned rather than aliased because Anthropic publishes no -latest alias
	// to point at instead.
	//
	// A pinned id is a decision with a shelf life, which is the argument the
	// Gemini block above makes for aliases. It applies here too: ANTHROPIC_MODEL
	// is how an operator moves the default on, and these three are what the
	// menu offers until somebody edits them.
	ModelClaudeOpus   = "claude-opus-5"
	ModelClaudeSonnet = "claude-sonnet-5"
	ModelClaudeHaiku  = "claude-haiku-4-5-20251001"
)

// DefaultModel is the model used for interactive queries (chat, agent).
// Anthropic wins when its key is set. OpenRouter is the interactive
// default only when it is the cloud provider in play — its slugs are
// provider/model, and a bare Claude id would 400.
func DefaultModel() string {
	if m := settings.Get("ANTHROPIC_MODEL"); m != "" {
		return m
	}
	// A preferred provider answers with its own model rather than being handed
	// a Claude id and having modelFor swap it. Same result, one less place
	// where the model on screen is not the model that ran.
	if p, _, _, ok := PreferredProvider(); ok && p != ProviderAnthropic {
		if m := PreferredModel(p, false); m != "" {
			return m
		}
	}
	if getOpenRouterAPIKey() != "" && settings.Get("ANTHROPIC_API_KEY") == "" {
		return OpenRouterModel()
	}
	// Current generation. This said claude-sonnet-4-6 — a generation behind,
	// pinned in a constant nobody revisits, on an instance whose operator
	// assumed it was getting the current model. A default model is a decision
	// with a shelf life; ANTHROPIC_MODEL is how an operator overrides it and
	// AGENT_MODEL is how they change only the agent's.
	return ModelClaudeSonnet
}

// AtlasModel is the model to send Atlas Cloud when the caller did not name one
// of theirs — the counterpart of OpenRouterModel, and for the same reason: a
// provider chosen as the last one available still has to be sent a model id it
// recognises.
func AtlasModel() string {
	if m := settings.Get("ATLAS_MODEL"); m != "" {
		return m
	}
	return ModelDeepSeekPro
}

// BackgroundModel is the model used for cheap background tasks
// (summaries, tags, moderation, topics).
func BackgroundModel() string {
	// The preferred provider's cheap end, when one is set.
	//
	// This reached for Atlas the moment an Atlas key existed, whatever the
	// instance preferred — which is how an Anthropic-first instance ended up
	// doing its summaries on DeepSeek without anybody choosing that.
	if p, _, _, ok := PreferredProvider(); ok {
		if m := PreferredModel(p, true); m != "" {
			return m
		}
	}
	if getAtlasAPIKey() != "" {
		return ModelDeepSeekFlash
	}
	if getOpenRouterAPIKey() != "" {
		return OpenRouterModel()
	}
	return ModelClaudeHaiku
}

// AtlasHosted reports whether a model id is one Atlas Cloud serves.
//
// Exported because the agent has to make the same judgement when an operator
// names a model in AGENT_MODEL: which provider a model belongs to is this
// package's knowledge, and a second copy of the prefix list in agent/ would be
// the kind that drifts.
func AtlasHosted(model string) bool { return isAtlasModel(model) }

// GeminiHosted reports whether a model id is one Google serves.
//
// Here rather than in agent/, for the reason AtlasHosted gives: which provider
// a model belongs to is this package's knowledge, and a second copy of the
// prefix list somewhere else is the kind that drifts. It matters more for
// Gemini than for Atlas — a Gemini id is a bare name with no slash in it, which
// is Anthropic's shape, so without this a run asked for gemini-2.5-pro is sent
// to Anthropic and answered with a 400.
func GeminiHosted(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "gemini")
}

// AtlasKey and GeminiKey are the keys, for callers outside this package.
//
// Exported so the agent asks rather than reading the environment itself. It
// read ATLAS_API_KEY directly in two places, which is how a variable gets
// renamed everywhere except the one path that matters.
func AtlasKey() string  { return getAtlasAPIKey() }
func GeminiKey() string { return getGeminiAPIKey() }

// isAtlasModel returns true if the model should be routed to Atlas Cloud.
func isAtlasModel(model string) bool {
	return strings.HasPrefix(model, "deepseek") ||
		strings.HasPrefix(model, "qwen") ||
		strings.HasPrefix(model, "Qwen") ||
		strings.HasPrefix(model, "glm") ||
		strings.HasPrefix(model, "kimi") ||
		strings.HasPrefix(model, "minimax")
}

// generate sends a prompt to the configured LLM provider
func generate(prompt *Prompt) (string, error) {
	// Acquire semaphore to limit concurrent requests
	ctx, cancel := context.WithTimeout(context.Background(), llmTimeout+5*time.Second)
	defer cancel()

	if err := llmSemaphore.Acquire(ctx, 1); err != nil {
		return "", fmt.Errorf("LLM request queue full, please try again later")
	}
	defer llmSemaphore.Release(1)

	systemPromptText, err := BuildSystemPrompt(prompt)
	if err != nil {
		return "", err
	}

	app.Log("ai", "[LLM] Question: %s", truncateLog(prompt.Question, 100))

	messages := []map[string]string{
		{"role": "system", "content": systemPromptText},
	}

	for _, v := range prompt.Context {
		messages = append(messages, map[string]string{"role": "user", "content": v.Prompt})
		messages = append(messages, map[string]string{"role": "assistant", "content": v.Answer})
	}

	messages = append(messages, map[string]string{"role": "user", "content": prompt.Question})

	model := prompt.Model
	if model == "" {
		model = DefaultModel()
	}

	caller := prompt.Caller
	if caller == "" {
		caller = "unknown"
	}

	maxTok := prompt.MaxTokens
	if maxTok == 0 {
		maxTok = 4096
	}

	// Every non-streaming model call goes through go-micro's ai package.
	return generateViaMicro(model, systemPromptText, messages, caller, maxTok)
}

func generateStream(prompt *Prompt, onToken func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmTimeout+5*time.Second)
	defer cancel()

	if err := llmSemaphore.Acquire(ctx, 1); err != nil {
		return "", fmt.Errorf("LLM request queue full, please try again later")
	}
	defer llmSemaphore.Release(1)

	systemPromptText, err := BuildSystemPrompt(prompt)
	if err != nil {
		return "", err
	}

	msgs := []map[string]string{
		{"role": "system", "content": systemPromptText},
	}
	for _, v := range prompt.Context {
		msgs = append(msgs, map[string]string{"role": "user", "content": v.Prompt})
		msgs = append(msgs, map[string]string{"role": "assistant", "content": v.Answer})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt.Question})

	mdl := prompt.Model
	if mdl == "" {
		mdl = DefaultModel()
	}

	clr := prompt.Caller
	if clr == "" {
		clr = "unknown"
	}

	maxTok := prompt.MaxTokens
	if maxTok == 0 {
		maxTok = 4096
	}

	// Stream through go-micro; it falls back to a single Generate for
	// providers that don't support streaming.
	return streamViaMicro(mdl, systemPromptText, msgs, clr, maxTok, onToken)
}

// CacheStats returns Anthropic prompt cache statistics
func CacheStats() (hits, misses, readTokens, creationTokens int) {
	cacheStatsMu.Lock()
	defer cacheStatsMu.Unlock()
	return cacheHits, cacheMisses, cacheReadTokens, cacheCreationTokens
}

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ── Local model support (Ollama, llama.cpp, vLLM, etc.) ──

// detectOllama checks if Ollama is running on the default port.
func detectOllama() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return ""
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		app.Log("ai", "Auto-detected Ollama at localhost:11434")
		return "http://localhost:11434/v1"
	}
	return ""
}

// detectLocalModel finds the best available model from a local server.
func detectLocalModel(baseURL, apiKey string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	// Prefer larger models, then any available
	preferred := []string{"llama3", "llama3.1", "llama3.2", "mistral", "qwen", "gemma", "phi"}
	for _, pref := range preferred {
		for _, m := range result.Data {
			if strings.Contains(strings.ToLower(m.ID), pref) {
				app.Log("ai", "Using local model: %s", m.ID)
				return m.ID
			}
		}
	}
	if len(result.Data) > 0 {
		app.Log("ai", "Using local model: %s", result.Data[0].ID)
		return result.Data[0].ID
	}
	return ""
}

// LocalModelAvailable returns true if a local model server is reachable.
func LocalModelAvailable() bool {
	if settings.Get("OPENAI_BASE_URL") != "" {
		return true
	}
	return detectOllama() != ""
}

func init() {
	// Inject cache stats function into app package to avoid import cycle
	app.CacheStatsFunc = CacheStats
}
