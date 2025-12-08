package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"mu/app"
)

type Model struct{}

func (m *Model) Generate(prompt *Prompt) (string, error) {
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

	return generateWithFanar(fanarAPIURL, fanarAPIKey, messages)
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

	client := &http.Client{}
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
func generateWithFanar(apiURL, apiKey string, messages []map[string]string) (string, error) {
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
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
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
