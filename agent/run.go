package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
)

// RunRequest is the input for the synchronous agent endpoint.
type RunRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

// RunResponse is the output of the synchronous agent endpoint.
type RunResponse struct {
	Answer string     `json:"answer"`
	Tools  []ToolUsed `json:"tools,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// ToolUsed records a tool call and its result.
type ToolUsed struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok" or "error"
}

// RunHandler handles POST /agent/run — synchronous agent query.
// Returns JSON with the answer instead of SSE streaming.
func RunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		app.RespondJSON(w, RunResponse{Error: "prompt required"})
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		w.WriteHeader(401)
		app.RespondJSON(w, RunResponse{Error: "authentication required"})
		return
	}

	// Check quota
	model := Models[0]
	for _, m := range Models {
		if m.ID == req.Model {
			model = m
			break
		}
	}
	if QuotaCheck != nil {
		canProceed, _, err := QuotaCheck(r, model.WalletOp)
		if !canProceed {
			w.WriteHeader(402)
			msg := "Insufficient credits"
			if err != nil {
				msg = err.Error()
			}
			app.RespondJSON(w, RunResponse{Error: msg})
			return
		}
	}

	_ = acc // authenticated

	// Step 1: Plan
	planResult, err := ai.Ask(&ai.Prompt{
		System: "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls.\n\n" +
			agentToolsDesc +
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. Output [] if no tools needed.",
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "agent-run-plan",
	})

	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	var toolCalls []toolCall

	if err == nil {
		planJSON := extractJSONArray(planResult)
		json.Unmarshal([]byte(planJSON), &toolCalls)
	}

	// Shortcut for common queries
	if len(toolCalls) == 0 {
		if tc := shortcutToolCalls(req.Prompt); len(tc) > 0 {
			for _, s := range tc {
				toolCalls = append(toolCalls, toolCall{Tool: s.Tool, Args: s.Args})
			}
		}
	}

	// Step 2: Execute tools
	var ragParts []string
	var toolsUsed []ToolUsed

	for _, tc := range toolCalls {
		if tc.Tool == "" {
			continue
		}
		text, isErr, execErr := api.ExecuteTool(r, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			toolsUsed = append(toolsUsed, ToolUsed{Name: tc.Tool, Status: "error"})
			continue
		}
		if len(text) > 8000 {
			text = text[:8000]
		}
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", tc.Tool, formatToolResult(tc.Tool, text, tc.Args)))
		toolsUsed = append(toolsUsed, ToolUsed{Name: tc.Tool, Status: "ok"})
	}

	// Step 3: Synthesise
	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	answer, err := ai.Ask(&ai.Prompt{
		System: "You are a helpful assistant. Today's date is " + today + ". " +
			"Answer using ONLY the tool results below. Use markdown.",
		Rag:      ragParts,
		Question: req.Prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "agent-run-synth",
	})
	if err != nil {
		app.RespondJSON(w, RunResponse{Error: err.Error(), Tools: toolsUsed})
		return
	}

	answer = app.StripLatexDollars(answer)
	app.RespondJSON(w, RunResponse{Answer: answer, Tools: toolsUsed})
}
