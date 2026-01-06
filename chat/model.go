package chat

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
	"mu/app"
)

// LLM request priorities
const (
	PriorityHigh   = 0 // User-facing chat
	PriorityMedium = 1 // Headlines/topic summaries  
	PriorityLow    = 2 // Background article summaries
)

var (
	// Limit concurrent LLM requests to prevent memory bloat when API is slow/rate-limited
	llmSemaphore = semaphore.NewWeighted(5)
	llmTimeout   = 60 * time.Second

	// Rate limiter for Fanar API (10 requests per minute)
	fanarRateMu      sync.Mutex
	fanarLastMinute  []time.Time
	fanarMaxPerMin   = 8 // Leave headroom below 10
	fanarHighPending int // Count of high-priority requests waiting
)

type Model struct{}

func (m *Model) Generate(prompt *Prompt) (string, error) {
	// Acquire semaphore to limit concurrent requests
	ctx, cancel := context.WithTimeout(context.Background(), llmTimeout+5*time.Second)
	defer cancel()

	if err := llmSemaphore.Acquire(ctx, 1); err != nil {
		return "", fmt.Errorf("LLM request queue full, please try again later")
	}
	defer llmSemaphore.Release(1)

	var systemPromptText string

	// Use provided system prompt or generate from template
	if prompt.System != "" {
		systemPromptText = prompt.System
	} else {
		sb := &strings.Builder{}
		if err := systemPrompt.Execute(sb, prompt); err != nil {
			return "", err
		}
		systemPromptText = sb.String()
	}

	// Debug: Show what's being sent to LLM
	app.Log("chat", "[LLM] System prompt:\n%s", systemPromptText)
	app.Log("chat", "[LLM] Question: %s", prompt.Question)

	messages := []map[string]string{
		map[string]string{
			"role":    "system",
			"content": systemPromptText,
		},
	}

	for _, v := range prompt.Context {
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": v.Prompt,
		})
		messages = append(messages, map[string]string{
			"role":    "assistant",
			"content": v.Answer,
		})
	}

	// add the question
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": prompt.Question,
	})

	// Check for Fanar configuration first
	fanarAPIKey := os.Getenv("FANAR_API_KEY")
	fanarAPIURL := os.Getenv("FANAR_API_URL")

	// Check for Anthropic configuration
	anthropicAPIKey := os.Getenv("ANTHROPIC_API_KEY")

	// Priority: Anthropic > Fanar > Ollama
	if anthropicAPIKey != "" {
		// Use Anthropic Claude
		modelName := os.Getenv("ANTHROPIC_MODEL")
		if modelName == "" {
			modelName = "claude-haiku-4.5-20250311" // Default to Claude Haiku 4.5
		}
		return generateWithAnthropic(anthropicAPIKey, modelName, systemPromptText, messages)
	}

	// Default to Ollama if Fanar is not configured
	if fanarAPIKey == "" {
		// Use Ollama as default
		modelName := os.Getenv("MODEL_NAME")
		if modelName == "" {
			modelName = "llama3.2" // Default Ollama model
		}

		apiURL := os.Getenv("MODEL_API_URL")
		if apiURL == "" {
			apiURL = "http://localhost:11434" // Default Ollama URL
		}

		return generateWithOllama(apiURL, modelName, messages)
	}

	// Use Fanar if API key is configured
	if fanarAPIURL == "" {
		fanarAPIURL = "https://api.fanar.qa" // Default Fanar URL
	}

	return generateWithFanar(fanarAPIURL, fanarAPIKey, messages, prompt.Priority)
}

// generateWithOllama generates a response using Ollama API
func generateWithOllama(apiURL, modelName string, messages []map[string]string) (string, error) {
	ollamaURL := fmt.Sprintf("%s/api/chat", apiURL)

	ollamaReq := map[string]interface{}{
		"model":    modelName,
		"messages": messages,
		"stream":   false,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return "", err
	}

	app.Log("chat", "[LLM] Using Ollama at %s with model %s", apiURL, modelName)

	httpReq, err := http.NewRequest("POST", ollamaURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Ollama: %v (make sure Ollama is running at %s)", err, apiURL)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("failed to parse Ollama response: %v", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	return ollamaResp.Message.Content, nil
}

// generateWithFanar generates a response using Fanar API
// checkFanarRateLimit returns true if we can make a request, false if rate limited
// priority: 0=high (chat), 1=medium (headlines), 2=low (background)
func checkFanarRateLimit(priority int) bool {
	fanarRateMu.Lock()
	defer fanarRateMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	// Remove requests older than 1 minute
	var recent []time.Time
	for _, t := range fanarLastMinute {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	fanarLastMinute = recent

	// Reserve slots for high-priority requests
	// Low priority can only use up to 4 slots, medium up to 6, high up to 8
	var maxForPriority int
	switch priority {
	case PriorityHigh:
		maxForPriority = fanarMaxPerMin
	case PriorityMedium:
		maxForPriority = 6
	case PriorityLow:
		maxForPriority = 4
	default:
		maxForPriority = 4
	}

	// Check if we're under the limit for this priority
	if len(fanarLastMinute) >= maxForPriority {
		return false
	}

	// Record this request
	fanarLastMinute = append(fanarLastMinute, now)
	return true
}

// GetFanarRateStatus returns current rate limit status for monitoring
func GetFanarRateStatus() (used, max int) {
	fanarRateMu.Lock()
	defer fanarRateMu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	count := 0
	for _, t := range fanarLastMinute {
		if t.After(cutoff) {
			count++
		}
	}
	return count, fanarMaxPerMin
}

func generateWithFanar(apiURL, apiKey string, messages []map[string]string, priority int) (string, error) {
	// Check rate limit before making request
	if !checkFanarRateLimit(priority) {
		// High priority waits longer, low priority fails fast
		maxWait := 3
		if priority == PriorityHigh {
			maxWait = 15
		} else if priority == PriorityMedium {
			maxWait = 8
		}

		app.Log("chat", "[LLM] Fanar rate limit reached (priority %d), waiting up to %ds...", priority, maxWait)
		for i := 0; i < maxWait; i++ {
			time.Sleep(time.Second)
			if checkFanarRateLimit(priority) {
				break
			}
			if i == maxWait-1 {
				if priority == PriorityLow {
					return "", fmt.Errorf("rate limit exceeded for background task, will retry later")
				}
				return "", fmt.Errorf("fanar rate limit exceeded, please try again in a minute")
			}
		}
	}

	fanarURL := fmt.Sprintf("%s/v1/chat/completions", apiURL)

	app.Log("chat", "[LLM] Using Fanar at %s", apiURL)

	fanarReq := map[string]interface{}{
		"model":    "Fanar",
		"messages": messages,
	}
	body, err := json.Marshal(fanarReq)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest("POST", fanarURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("fanar API request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var fanarResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &fanarResp); err != nil {
		return "", err
	}

	var content string
	if len(fanarResp.Choices) > 0 {
		content = fanarResp.Choices[0].Message.Content
	}
	if fanarResp.Error != nil {
		return "", fmt.Errorf("%v", fanarResp.Error)
	}
	return content, nil
}
// generateWithAnthropic generates a response using Anthropic Claude API
func generateWithAnthropic(apiKey, modelName, systemPrompt string, messages []map[string]string) (string, error) {
	apiURL := "https://api.anthropic.com/v1/messages"

	app.Log("chat", "[LLM] Using Anthropic Claude with model %s", modelName)

	// Anthropic API expects a different format - extract user/assistant messages
	// and use the systemPrompt separately
	var anthropicMessages []map[string]string
	for _, msg := range messages {
		// Skip system messages as we handle them separately
		if msg["role"] == "system" {
			continue
		}
		anthropicMessages = append(anthropicMessages, msg)
	}

	anthropicReq := map[string]interface{}{
		"model":      modelName,
		"max_tokens": 4096,
		"messages":   anthropicMessages,
	}

	// Add system prompt if provided
	if systemPrompt != "" {
		anthropicReq["system"] = systemPrompt
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Anthropic API: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var anthropicResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return "", fmt.Errorf("failed to parse Anthropic response: %v", err)
	}

	if anthropicResp.Error.Message != "" {
		return "", fmt.Errorf("anthropic error: %s", anthropicResp.Error.Message)
	}

	var content string
	for _, c := range anthropicResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	if content == "" {
		return "", fmt.Errorf("no content in Anthropic response")
	}

	return content, nil
}