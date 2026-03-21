package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"mu/internal/app"
)

var (
	// Limit concurrent LLM requests to prevent memory bloat
	llmSemaphore = semaphore.NewWeighted(5)
	llmTimeout   = 60 * time.Second

	// Anthropic cache stats
	cacheStatsMu        sync.Mutex
	cacheHits           int
	cacheMisses         int
	cacheReadTokens     int
	cacheCreationTokens int

	// Per-caller token usage tracking
	usageMu sync.Mutex
	usage   = map[string]*CallerUsage{}
)

// CallerUsage tracks token consumption for a specific caller.
type CallerUsage struct {
	Calls        int   `json:"calls"`
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	CacheRead    int   `json:"cache_read_tokens"`
	CacheWrite   int   `json:"cache_write_tokens"`
	LastCall     int64 `json:"last_call_unix"`
}

func recordUsage(caller string, input, output, cacheRead, cacheWrite int) {
	if caller == "" {
		caller = "unknown"
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	u, ok := usage[caller]
	if !ok {
		u = &CallerUsage{}
		usage[caller] = u
	}
	u.Calls++
	u.InputTokens += input
	u.OutputTokens += output
	u.CacheRead += cacheRead
	u.CacheWrite += cacheWrite
	u.LastCall = time.Now().Unix()
}

// GetUsageStats returns per-caller token usage since startup.
func GetUsageStats() map[string]*CallerUsage {
	usageMu.Lock()
	defer usageMu.Unlock()
	cp := make(map[string]*CallerUsage, len(usage))
	for k, v := range usage {
		clone := *v
		cp[k] = &clone
	}
	return cp
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

	caller := prompt.Caller
	if caller == "" {
		caller = "unknown"
	}
	app.Log("ai", "[LLM] [%s] Question: %s", caller, truncateLog(prompt.Question, 100))

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
		model = os.Getenv("ANTHROPIC_MODEL")
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	maxTokens := prompt.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	return generateAnthropic(key, model, systemPromptText, messages, caller, maxTokens)
}

func generateAnthropic(apiKey, model, systemPrompt string, messages []map[string]string, caller string, maxTokens int) (string, error) {
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
		"max_tokens": maxTokens,
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
		app.Log("ai", "[LLM] [%s] Cache HIT: %d cached, %d input, %d output tokens",
			caller, result.Usage.CacheReadInputTokens, result.Usage.InputTokens, result.Usage.OutputTokens)
	} else if result.Usage.CacheCreationInputTokens > 0 {
		cacheMisses++
		cacheCreationTokens += result.Usage.CacheCreationInputTokens
		app.Log("ai", "[LLM] [%s] Cache WRITE: %d cached, %d input, %d output tokens",
			caller, result.Usage.CacheCreationInputTokens, result.Usage.InputTokens, result.Usage.OutputTokens)
	} else {
		app.Log("ai", "[LLM] [%s] %d input, %d output tokens",
			caller, result.Usage.InputTokens, result.Usage.OutputTokens)
	}
	cacheStatsMu.Unlock()

	// Track per-caller usage
	recordUsage(caller, result.Usage.InputTokens, result.Usage.OutputTokens,
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

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func init() {
	// Inject stats functions into app package to avoid import cycle
	app.CacheStatsFunc = GetCacheStats
	app.UsageStatsFunc = func() map[string]*app.CallerUsage {
		raw := GetUsageStats()
		out := make(map[string]*app.CallerUsage, len(raw))
		for k, v := range raw {
			out[k] = &app.CallerUsage{
				Calls:        v.Calls,
				InputTokens:  v.InputTokens,
				OutputTokens: v.OutputTokens,
				CacheRead:    v.CacheRead,
				CacheWrite:   v.CacheWrite,
				LastCall:     v.LastCall,
			}
		}
		return out
	}
}
