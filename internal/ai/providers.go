package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func generateAnthropic(apiKey, model, systemPrompt string, messages []map[string]string, caller string) (string, error) {
	return generateAnthropicInternal(apiKey, model, systemPrompt, messages, caller, nil, 4096)
}

func generateAnthropicInternal(apiKey, model, systemPrompt string, messages []map[string]string, caller string, onToken func(string), maxTokens int) (string, error) {
	app.Log("ai", "[LLM] Using Anthropic Claude with model %s (stream=%v)", model, onToken != nil)

	var anthropicMessages []map[string]interface{}
	for _, msg := range messages {
		if msg["role"] != "system" {
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    msg["role"],
				"content": msg["content"],
			})
		}
	}

	reqBody := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   anthropicMessages,
	}
	if onToken != nil {
		reqBody["stream"] = true
	}

	if systemPrompt != "" {
		reqBody["system"] = []map[string]interface{}{
			{
				"type": "text",
				"text": systemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		}
	}

	body, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	client := &http.Client{Timeout: llmTimeout}
	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("anthropic", "POST", "api.anthropic.com/v1/messages", 0, duration, err, "", "")
		return "", fmt.Errorf("failed to connect to Anthropic: %v", err)
	}
	defer resp.Body.Close()

	if onToken != nil {
		return readAnthropicStream(resp, caller, model, start, onToken)
	}

	respBody, _ := io.ReadAll(resp.Body)
	app.RecordAPICall("anthropic", "POST", "api.anthropic.com/v1/messages ["+model+"]", resp.StatusCode, duration, nil, "", "")

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

	cacheStatsMu.Lock()
	if result.Usage.CacheReadInputTokens > 0 {
		cacheHits++
		cacheReadTokens += result.Usage.CacheReadInputTokens
	} else if result.Usage.CacheCreationInputTokens > 0 {
		cacheMisses++
		cacheCreationTokens += result.Usage.CacheCreationInputTokens
	}
	cacheStatsMu.Unlock()

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

func readAnthropicStream(resp *http.Response, caller, model string, start time.Time, onToken func(string)) (string, error) {
	defer resp.Body.Close()

	duration := time.Since(start)
	app.RecordAPICall("anthropic", "POST", "api.anthropic.com/v1/messages ["+model+"] (stream)", resp.StatusCode, duration, nil, "", "")

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic stream error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var full strings.Builder
	var inputTokens, outputTokens, cacheRead, cacheWrite int
	buf := make([]byte, 4096)
	var lineBuf strings.Builder

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			lineBuf.Write(buf[:n])
			for {
				text := lineBuf.String()
				idx := strings.Index(text, "\n")
				if idx < 0 {
					break
				}
				line := text[:idx]
				lineBuf.Reset()
				lineBuf.WriteString(text[idx+1:])

				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := line[6:]
				if data == "[DONE]" {
					continue
				}

				var ev struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						OutputTokens             int `json:"output_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				}
				json.Unmarshal([]byte(data), &ev)

				switch ev.Type {
				case "content_block_delta":
					if ev.Delta.Text != "" {
						full.WriteString(ev.Delta.Text)
						onToken(ev.Delta.Text)
					}
				case "message_start":
					inputTokens = ev.Usage.InputTokens
					cacheRead = ev.Usage.CacheReadInputTokens
					cacheWrite = ev.Usage.CacheCreationInputTokens
				case "message_delta":
					outputTokens = ev.Usage.OutputTokens
				}
			}
		}
		if err != nil {
			break
		}
	}

	cacheStatsMu.Lock()
	if cacheRead > 0 {
		cacheHits++
		cacheReadTokens += cacheRead
	} else if cacheWrite > 0 {
		cacheMisses++
		cacheCreationTokens += cacheWrite
	}
	cacheStatsMu.Unlock()

	recordUsage(caller, model, inputTokens, outputTokens, cacheRead, cacheWrite)
	app.Log("ai", "[LLM] Usage [%s]: input=%d output=%d cache_read=%d cache_write=%d (streamed)",
		caller, inputTokens, outputTokens, cacheRead, cacheWrite)

	return full.String(), nil
}

// GetCacheStats returns Anthropic prompt cache statistics
func GetCacheStats() (hits, misses, readTokens, creationTokens int) {
	cacheStatsMu.Lock()
	defer cacheStatsMu.Unlock()
	return cacheHits, cacheMisses, cacheReadTokens, cacheCreationTokens
}

// generateAtlas sends a request to Atlas Cloud's OpenAI-compatible API.
func generateAtlas(apiKey, model, systemPrompt string, messages []map[string]string, caller string, maxTokens ...int) (string, error) {
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

	// Use shorter max_tokens for background tasks to reduce latency.
	tokLimit := 4096
	if len(maxTokens) > 0 && maxTokens[0] > 0 {
		tokLimit = maxTokens[0]
	}
	switch caller {
	case "article-summary", "auto-tag-post", "auto-tag-note", "topic-generation", "topic-summary":
		tokLimit = 512
	}

	req := map[string]interface{}{
		"model":      model,
		"messages":   apiMessages,
		"max_tokens": tokLimit,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", getAtlasBaseURL()+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: llmTimeout}
	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("atlas", "POST", "api.atlascloud.ai/v1/chat/completions", 0, duration, err, "", "")
		return "", fmt.Errorf("atlas cloud connection failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	app.RecordAPICall("atlas", "POST", "api.atlascloud.ai ["+model+"]", resp.StatusCode, duration, nil, "", "")

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

// generateLocalOpenAI sends a request to a local OpenAI-compatible server.
func generateLocalOpenAI(baseURL, apiKey, model, systemPrompt string, messages []map[string]string, caller string, maxTokens int) (string, error) {
	app.Log("ai", "[LLM] Using local model %s at %s", model, baseURL)

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

	reqBody := map[string]interface{}{
		"model":      model,
		"messages":   apiMessages,
		"max_tokens": maxTokens,
	}

	body, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: llmTimeout}
	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		app.RecordAPICall("local", "POST", baseURL, 0, duration, err, "", "")
		return "", fmt.Errorf("local model connection failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	app.RecordAPICall("local", "POST", baseURL+" ["+model+"]", resp.StatusCode, duration, nil, "", "")

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
		return "", fmt.Errorf("local model error: %s", result.Error.Message)
	}

	recordUsage(caller, model, result.Usage.PromptTokens, result.Usage.CompletionTokens, 0, 0)

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("local model returned no choices")
	}
	return result.Choices[0].Message.Content, nil
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
