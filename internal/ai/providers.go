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

func getAtlasAPIKey() string {
	if v := settings.Get("ATLAS_API_KEY"); v != "" {
		return v
	}
	return settings.Get("OPENAI_API_KEY")
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
	if settings.Get("OPENAI_BASE_URL") != "" {
		return true
	}
	return detectOllama() != ""
}

func getAtlasBaseURL() string {
	if v := settings.Get("OPENAI_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.atlascloud.ai/v1"
}

// Atlas Cloud model aliases — used to route requests to Atlas Cloud
// instead of Anthropic. Any model string starting with "deepseek" or
// "qwen" is routed to Atlas Cloud automatically.
const (
	ModelDeepSeekPro   = "deepseek-ai/deepseek-v4-pro"
	ModelDeepSeekFlash = "deepseek-ai/deepseek-v4-flash"
	ModelQwenPlus      = "qwen/qwen3.6-plus"
)

// DefaultModel is the model used for interactive queries (chat, agent).
// Always uses Anthropic for speed — Atlas Cloud is for background only.
func DefaultModel() string {
	m := settings.Get("ANTHROPIC_MODEL")
	if m != "" {
		return m
	}
	return "claude-sonnet-4-6"
}

// BackgroundModel is the model used for cheap background tasks
// (summaries, tags, moderation, topics).
func BackgroundModel() string {
	if getAtlasAPIKey() != "" {
		return ModelDeepSeekFlash
	}
	return "claude-haiku-4-5-20251001"
}

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

// GetCacheStats returns Anthropic prompt cache statistics
func GetCacheStats() (hits, misses, readTokens, creationTokens int) {
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
	app.CacheStatsFunc = GetCacheStats
}
