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
)

var DefaultModel = "gpt-4o-mini"

// Model implements the ModelHandler interface
type Model struct{}

// Generate calls the OpenAI or Gemini API with the selected model and prompt
func (m *Model) Generate(ctx context.Context, prompt *Prompt) (string, error) {
	sb := &strings.Builder{}
	if err := systemPrompt.Execute(sb, prompt.Context); err != nil {
		return "", err
	}

	// Supported models
	openaiModels := map[string]bool{
		"Fanar":         true,
		"gpt-4o-mini":   true,
		"gpt-4-turbo":   true,
		"gpt-3.5-turbo": true,
	}
	geminiModels := map[string]string{
		"gemini-2.5-flash-lite": "gemini-2.5-flash-lite",
		"gemini-2.5-flash":      "gemini-2.5-flash",
		"gemini-2.5-pro":        "gemini-2.5-pro",
	}

	if openaiModels[prompt.Model] {
		openaiURL := "https://api.openai.com/v1/chat/completions"
		apiKey := os.Getenv("OPENAI_API_KEY")

		if prompt.Model == "Fanar" {
			openaiURL = "https://api.fanar.qa/v1/chat/completions"
			apiKey = os.Getenv("FANAR_API_KEY")
		}

		if apiKey == "" {
			return "", fmt.Errorf("OPENAI_API_KEY not set")
		}
		openaiReq := map[string]interface{}{
			"model": prompt.Model,
			"messages": []map[string]string{
				{"role": "system", "content": sb.String()},
				{"role": "user", "content": prompt.Question},
			},
		}
		body, err := json.Marshal(openaiReq)
		if err != nil {
			return "", err
		}
		httpReq, err := http.NewRequest("POST", openaiURL, bytes.NewReader(body))
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
		var openaiResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error interface{} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &openaiResp); err != nil {
			return "", err
		}

		var content string
		if len(openaiResp.Choices) > 0 {
			content = openaiResp.Choices[0].Message.Content
		}
		if openaiResp.Error != nil {
			return "", fmt.Errorf("%v", openaiResp.Error)
		}
		return content, nil
	}

	if apiModel, ok := geminiModels[prompt.Model]; ok {
		geminiURL := "https://generativelanguage.googleapis.com/v1beta/models/" + apiModel + ":generateContent?key=" + os.Getenv("GEMINI_API_KEY")
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return "", fmt.Errorf("GEMINI_API_KEY not set")
		}
		geminiReq := map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": prompt.Question}}},
			},
		}
		body, err := json.Marshal(geminiReq)
		if err != nil {
			return "", err
		}
		resp, err := http.Post(geminiURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			Error interface{} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &geminiResp); err != nil {
			return "", err
		}

		var content string

		if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
			content = geminiResp.Candidates[0].Content.Parts[0].Text
		}

		if geminiResp.Error != nil {
			return "", fmt.Errorf("%v", geminiResp.Error)
		}

		return content, nil
	}

	return "", fmt.Errorf("invalid model: %s", prompt.Model)
}
