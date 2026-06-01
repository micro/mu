package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"mu/internal/app"
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

	// Atlas Cloud config
	atlasAPIKey = os.Getenv("ATLAS_API_KEY")
	atlasBaseURL = "https://api.atlascloud.ai/v1"
)

// Atlas Cloud model aliases — used to route requests to Atlas Cloud
// instead of Anthropic. Any model string starting with "deepseek" or
// "qwen" is routed to Atlas Cloud automatically.
const (
	ModelDeepSeekPro   = "deepseek-ai/deepseek-v4-pro"
	ModelDeepSeekFlash = "deepseek-ai/deepseek-v4-flash"
	ModelQwenPlus      = "qwen/qwen3.6-plus"
)

// DefaultModel is the model used for interactive queries (chat, agent).
// Falls back to Anthropic Sonnet if Atlas Cloud is not configured.
func DefaultModel() string {
	if atlasAPIKey != "" {
		return ModelDeepSeekPro
	}
	m := os.Getenv("ANTHROPIC_MODEL")
	if m != "" {
		return m
	}
	return "claude-sonnet-4-20250514"
}

// BackgroundModel is the model used for cheap background tasks
// (summaries, tags, moderation, topics).
func BackgroundModel() string {
	if atlasAPIKey != "" {
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

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	model := prompt.Model
	if model == "" {
		model = DefaultModel()
	}

	caller := prompt.Caller
	if caller == "" {
		caller = "unknown"
	}

	// Route to Atlas Cloud for supported models.
	if isAtlasModel(model) && atlasAPIKey != "" {
		return generateAtlas(atlasAPIKey, model, systemPromptText, messages, caller)
	}

	return generateAnthropic(key, model, systemPromptText, messages, caller)
}

func generateAnthropic(apiKey, model, systemPrompt string, messages []map[string]string, caller string) (string, error) {
	app.Log("ai", "[LLM] Using Anthropic Claude with model %s", model)

	var anthropicMessages []map[string]interface{}
	for _, msg := range messages {
		if msg["role"] != "system" {
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    msg["role"],
				"content": msg["content"],
			})
		}
	}

	req := map[string]interface{}{
		"model":      model,
		"max_tokens": 4096,
		"messages":   anthropicMessages,
	}

	// Use array format for system prompt with cache_control for prompt caching
	// This caches the system prompt for 5+ minutes, saving ~90% on repeated calls
	if systemPrompt != "" {
		req["system"] = []map[string]interface{}{
			{
				"type": "text",
				"text": systemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		}
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Anthropic: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error.Message != "" {
		return "", fmt.Errorf("anthropic error: %s", result.Error.Message)
	}

	// Track and log cache status
	cacheStatsMu.Lock()
	if result.Usage.CacheReadInputTokens > 0 {
		cacheHits++
		cacheReadTokens += result.Usage.CacheReadInputTokens
		app.Log("ai", "[LLM] Cache HIT: %d tokens from cache, %d new input tokens",
			result.Usage.CacheReadInputTokens, result.Usage.InputTokens)
	} else if result.Usage.CacheCreationInputTokens > 0 {
		cacheMisses++
		cacheCreationTokens += result.Usage.CacheCreationInputTokens
		app.Log("ai", "[LLM] Cache WRITE: %d tokens cached for future requests",
			result.Usage.CacheCreationInputTokens)
	}
	cacheStatsMu.Unlock()

	// Record usage for tracking
	recordUsage(caller, model,
		result.Usage.InputTokens, result.Usage.OutputTokens,
		result.Usage.CacheReadInputTokens, result.Usage.CacheCreationInputTokens)

	app.Log("ai", "[LLM] Usage [%s]: input=%d output=%d cache_read=%d cache_write=%d",
		caller, result.Usage.InputTokens, result.Usage.OutputTokens,
		result.Usage.CacheReadInputTokens, result.Usage.CacheCreationInputTokens)

	var content string
	for _, c := range result.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}
	return content, nil
}

// GetCacheStats returns Anthropic prompt cache statistics
func GetCacheStats() (hits, misses, readTokens, creationTokens int) {
	cacheStatsMu.Lock()
	defer cacheStatsMu.Unlock()
	return cacheHits, cacheMisses, cacheReadTokens, cacheCreationTokens
}

// generateAtlas sends a request to Atlas Cloud's OpenAI-compatible API.
func generateAtlas(apiKey, model, systemPrompt string, messages []map[string]string, caller string) (string, error) {
	app.Log("ai", "[LLM] Using Atlas Cloud with model %s", model)

	var apiMessages []map[string]string
	if systemPrompt != "" {
		apiMessages = append(apiMessages, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	for _, msg := range messages {
		if msg["role"] != "system" {
			apiMessages = append(apiMessages, msg)
		}
	}

	req := map[string]interface{}{
		"model":      model,
		"messages":   apiMessages,
		"max_tokens": 4096,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", atlasBaseURL+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("atlas cloud connection failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error.Message != "" {
		return "", fmt.Errorf("atlas cloud error: %s", result.Error.Message)
	}

	// Record usage.
	recordUsage(caller, model,
		result.Usage.PromptTokens, result.Usage.CompletionTokens, 0, 0)

	app.Log("ai", "[LLM] Usage [%s]: input=%d output=%d (atlas cloud)",
		caller, result.Usage.PromptTokens, result.Usage.CompletionTokens)

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("atlas cloud returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func init() {
	// Inject cache stats function into app package to avoid import cycle
	app.CacheStatsFunc = GetCacheStats
}
