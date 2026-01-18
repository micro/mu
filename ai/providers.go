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
	"mu/app"
)

var (
	// Limit concurrent LLM requests to prevent memory bloat
	llmSemaphore = semaphore.NewWeighted(5)
	llmTimeout   = 60 * time.Second

	// Rate limiter for Fanar API
	fanarRateMu     sync.Mutex
	fanarLastMinute []time.Time
	fanarMaxPerMin  = 35
)

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

	// Provider priority: Anthropic > Fanar > Ollama
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-haiku-4-20250514"
		}
		return generateAnthropic(key, model, systemPromptText, messages)
	}

	if key := os.Getenv("FANAR_API_KEY"); key != "" {
		url := os.Getenv("FANAR_API_URL")
		if url == "" {
			url = "https://api.fanar.qa"
		}
		return generateFanar(url, key, messages, prompt.Priority)
	}

	// Default to Ollama
	model := os.Getenv("MODEL_NAME")
	if model == "" {
		model = "llama3.2"
	}
	url := os.Getenv("MODEL_API_URL")
	if url == "" {
		url = "http://localhost:11434"
	}
	return generateOllama(url, model, messages)
}

func generateOllama(apiURL, model string, messages []map[string]string) (string, error) {
	app.Log("ai", "[LLM] Using Ollama at %s with model %s", apiURL, model)

	req := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", apiURL+"/api/chat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Ollama: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error != "" {
		return "", fmt.Errorf("ollama error: %s", result.Error)
	}
	return result.Message.Content, nil
}

func generateFanar(apiURL, apiKey string, messages []map[string]string, priority int) (string, error) {
	if !checkFanarRateLimit(priority) {
		maxWait := 3
		if priority == PriorityHigh {
			maxWait = 15
		} else if priority == PriorityMedium {
			maxWait = 8
		}

		app.Log("ai", "[LLM] Fanar rate limit reached (priority %d), waiting...", priority)
		for i := 0; i < maxWait; i++ {
			time.Sleep(time.Second)
			if checkFanarRateLimit(priority) {
				break
			}
			if i == maxWait-1 {
				return "", fmt.Errorf("fanar rate limit exceeded")
			}
		}
	}

	app.Log("ai", "[LLM] Using Fanar at %s", apiURL)

	req := map[string]interface{}{
		"model":    "Fanar",
		"messages": messages,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", apiURL+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: llmTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("fanar API request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error interface{} `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error != nil {
		return "", fmt.Errorf("%v", result.Error)
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", nil
}

func generateAnthropic(apiKey, model, systemPrompt string, messages []map[string]string) (string, error) {
	app.Log("ai", "[LLM] Using Anthropic Claude with model %s", model)

	var anthropicMessages []map[string]string
	for _, msg := range messages {
		if msg["role"] != "system" {
			anthropicMessages = append(anthropicMessages, msg)
		}
	}

	req := map[string]interface{}{
		"model":      model,
		"max_tokens": 4096,
		"messages":   anthropicMessages,
	}
	if systemPrompt != "" {
		req["system"] = systemPrompt
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

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
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error.Message != "" {
		return "", fmt.Errorf("anthropic error: %s", result.Error.Message)
	}

	var content string
	for _, c := range result.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}
	return content, nil
}

func checkFanarRateLimit(priority int) bool {
	fanarRateMu.Lock()
	defer fanarRateMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	var recent []time.Time
	for _, t := range fanarLastMinute {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	fanarLastMinute = recent

	var maxForPriority int
	switch priority {
	case PriorityHigh:
		maxForPriority = fanarMaxPerMin
	case PriorityMedium:
		maxForPriority = 25
	default:
		maxForPriority = 15
	}

	if len(fanarLastMinute) >= maxForPriority {
		return false
	}

	fanarLastMinute = append(fanarLastMinute, now)
	return true
}

// GetFanarRateStatus returns current rate limit status
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

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
