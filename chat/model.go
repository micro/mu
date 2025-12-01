package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Model struct{}

func (m *Model) Generate(prompt *Prompt) (string, error) {
	var systemPromptText string

	// Use provided system prompt or generate from template
	if prompt.System != "" {
		systemPromptText = prompt.System
	} else {
		sb := &strings.Builder{}
		if err := systemPrompt.Execute(sb, prompt.Rag); err != nil {
			return "", err
		}
		systemPromptText = sb.String()
	}

	// Debug: Show what's being sent to LLM
	fmt.Printf("[LLM] System prompt:\n%s\n", systemPromptText)
	fmt.Printf("[LLM] Question: %s\n", prompt.Question)

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

	apiKey := os.Getenv("FANAR_API_KEY")
	apiURL := "https://api.fanar.qa"
	fanarURL := fmt.Sprintf("%s/v1/chat/completions", apiURL)

	if len(apiKey) == 0 {
		return "", fmt.Errorf("FANAR_API_KEY not set")
	}

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
