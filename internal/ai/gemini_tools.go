package ai

// Mu's Gemini tool continuation adapter. Based on go-micro's Gemini provider;
// keep the signed response parts intact and report failed follow-up calls.
// The underlying provider still owns configuration and the plain text stream.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-micro.dev/v6/model"
	"go-micro.dev/v6/model/gemini"
	"io"
	"net/http"
	"strings"
)

type geminiTools struct{ *gemini.Provider }

type geminiFunctionCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func geminiToolValue(result model.ToolResult) any {
	if result.Refused != "" {
		return map[string]any{"error": result.Refused, "message": result.Content}
	}
	value := result.Value
	if value == nil && result.Content != "" {
		if json.Unmarshal([]byte(result.Content), &value) != nil {
			value = result.Content
		}
	}
	// Gemini requires an object even when a tool returns text, a list or null.
	raw, err := json.Marshal(value)
	var object map[string]any
	if err == nil && json.Unmarshal(raw, &object) == nil && object != nil {
		return object
	}
	return map[string]any{"result": value}
}
func (p *geminiTools) Generate(ctx context.Context, req *model.Request, opts ...model.GenerateOption) (*model.Response, error) {
	var tools []map[string]any
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": t.Properties,
			},
		})
	}

	contents := geminiToolContents(req)

	apiReq := map[string]any{
		"contents": contents,
	}

	if req.SystemPrompt != "" {
		apiReq["system_instruction"] = map[string]any{
			"parts": []map[string]any{{"text": req.SystemPrompt}},
		}
	}

	if len(tools) > 0 {
		apiReq["tools"] = []map[string]any{
			{"functionDeclarations": tools},
		}
	}

	resp, rawParts, err := p.callAPI(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	if len(resp.ToolCalls) == 0 {
		return resp, nil
	}

	// Tool execution loop: execute tools, send results back, and keep the
	// function declarations on offer so the model can take the next step. A
	// follow-up without tools asks the model to continue with its hands tied —
	// the call it wanted comes back written out as prose — and without a loop
	// a second step is impossible whatever the model wants. Bounded so a model
	// that never stops asking cannot run forever.
	if p.Options().ToolHandler != nil {
		// Copied rather than aliased: append on a slice that shares an array
		// with contents would overwrite it on a later round.
		followUpContents := append([]map[string]any(nil), contents...)
		pending := resp.ToolCalls
		raw := rawParts
		for round := 0; len(pending) > 0 && round < 12; round++ {
			var resultParts []map[string]any
			for _, tc := range pending {
				result := geminiToolValue(p.Options().ToolHandler(ctx, tc))
				resultParts = append(resultParts, map[string]any{
					"functionResponse": map[string]any{
						"name":     tc.Name,
						"id":       tc.ID,
						"response": result,
					},
				})
			}

			followUpContents = append(followUpContents,
				map[string]any{"role": "model", "parts": raw},
				map[string]any{"role": "user", "parts": resultParts},
			)

			followUpReq := map[string]any{
				"contents": followUpContents,
			}
			if req.SystemPrompt != "" {
				followUpReq["system_instruction"] = map[string]any{
					"parts": []map[string]any{{"text": req.SystemPrompt}},
				}
			}
			if len(tools) > 0 {
				followUpReq["tools"] = []map[string]any{
					{"functionDeclarations": tools},
				}
			}

			followUpResp, followUpRaw, err := p.callAPI(ctx, followUpReq)
			if err != nil {
				return nil, fmt.Errorf("gemini tool follow-up: %w", err)
			}
			if followUpResp.Reply != "" {
				resp.Answer = followUpResp.Reply
			}
			pending, raw = followUpResp.ToolCalls, followUpRaw
			resp.ToolCalls = append(resp.ToolCalls, followUpResp.ToolCalls...)
			resp.Usage.InputTokens += followUpResp.Usage.InputTokens
			resp.Usage.OutputTokens += followUpResp.Usage.OutputTokens
			resp.Usage.TotalTokens += followUpResp.Usage.TotalTokens
		}
		if len(pending) > 0 {
			return nil, fmt.Errorf("gemini exceeded tool round limit")
		}
	}

	return resp, nil
}

func (p *geminiTools) callAPI(ctx context.Context, req map[string]any) (*model.Response, []json.RawMessage, error) {
	if cap := p.Options().MaxTokens; cap > 0 {
		req["generationConfig"] = map[string]any{"maxOutputTokens": cap}
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := strings.TrimRight(p.Options().BaseURL, "/") +
		"/v1beta/models/" + p.Options().Model + ":generateContent"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.Options().APIKey)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("API request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return nil, nil, model.NewHTTPError(httpResp, respBody)
	}

	var geminiResp struct {
		Usage struct {
			Prompt     int `json:"promptTokenCount"`
			Candidates int `json:"candidatesTokenCount"`
			Thoughts   int `json:"thoughtsTokenCount"`
			Total      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
		Candidates []struct {
			Content struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, nil, fmt.Errorf("no response from API")
	}

	parts := geminiResp.Candidates[0].Content.Parts
	response := &model.Response{Usage: model.Usage{InputTokens: geminiResp.Usage.Prompt, OutputTokens: geminiResp.Usage.Candidates + geminiResp.Usage.Thoughts, TotalTokens: geminiResp.Usage.Total}}

	var replyParts []string
	var rawParts []json.RawMessage

	for _, raw := range parts {
		var part struct {
			Text         string              `json:"text"`
			Thought      bool                `json:"thought"`
			FunctionCall *geminiFunctionCall `json:"functionCall"`
		}
		if err := json.Unmarshal(raw, &part); err != nil {
			return nil, nil, err
		}
		// Return each signed part intact; thought text is never user output.
		rawParts = append(rawParts, raw)
		if part.Text != "" && !part.Thought {
			replyParts = append(replyParts, part.Text)
		}
		if part.FunctionCall != nil {
			response.ToolCalls = append(response.ToolCalls, model.ToolCall{ID: part.FunctionCall.ID, Name: part.FunctionCall.Name, Input: part.FunctionCall.Args})
		}
	}

	if len(replyParts) > 0 {
		response.Reply = strings.Join(replyParts, "\n")
	}

	return response, rawParts, nil
}

func geminiToolContents(req *model.Request) []map[string]any {
	contents := make([]map[string]any, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role == "system" || role == "" {
			continue
		}
		contents = append(contents, map[string]any{"role": role, "parts": []map[string]any{{"text": fmt.Sprint(m.Content)}}})
	}
	if req.Prompt != "" {
		contents = append(contents, map[string]any{"role": "user", "parts": []map[string]any{{"text": req.Prompt}}})
	}
	return contents
}
