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
	"mu/internal/memory"
)

// extractMemory checks if the user's prompt contains something to
// remember (preferences, facts about themselves, interests). Runs
// async after the response so it doesn't slow down the answer.
func extractMemory(accountID, prompt string) {
	lower := strings.ToLower(prompt)
	// Quick check — only run the LLM if the prompt looks like it
	// contains a memory-worthy statement.
	triggers := []string{"remember", "my ", "i like", "i prefer", "i'm ", "i am ",
		"don't show", "always ", "never ", "i want", "i need", "i use", "my name",
		"call me", "i live", "i work"}
	found := false
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			found = true
			break
		}
	}
	if !found {
		return
	}

	result, err := ai.Ask(&ai.Prompt{
		System: `Extract any personal preference or fact the user is sharing about themselves.
Output ONLY valid JSON: {"key":"short label","value":"what to remember"}
If the message does NOT contain a personal preference or fact, output: {}
Examples:
"Remember I like Bitcoin" → {"key":"interest","value":"likes Bitcoin"}
"I live in London" → {"key":"location","value":"London"}
"What's the weather?" → {}`,
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Caller:   "memory-extract",
	})
	if err != nil || result == "" {
		return
	}
	var extracted struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &extracted); err != nil {
		return
	}
	if extracted.Key != "" && extracted.Value != "" {
		memory.Set(accountID, extracted.Key, extracted.Value)
		app.Log("memory", "Saved for %s: %s = %s", accountID, extracted.Key, extracted.Value)
	}
}

// UserContextFunc is set by main.go to provide personalised context
// for the agent's responses. Returns a string with the user's current
// state (unread mail, market prices, etc.) that gets injected into the
// synthesis prompt.
var UserContextFunc func(accountID string) string
type RunRequest struct {
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
	ContextID string `json:"context_id"` // prior flow ID for follow-ups
}

// RunResponse is the output of the synchronous agent endpoint.
type RunResponse struct {
	Answer string     `json:"answer"`
	FlowID string     `json:"flow_id,omitempty"`
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
	userCtx := ""
	if UserContextFunc != nil {
		userCtx = UserContextFunc(acc.ID)
	}
	planSystem := "You are an AI agent. Given a user question, output ONLY a JSON array of tool calls.\n\n" +
		agentToolsDesc +
		"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. Output [] if no tools needed." +
		"\n\nIMPORTANT: For personal questions like 'do I have mail', 'what's the weather', 'news today', 'btc price' — ALWAYS use the appropriate tool. Never say you can't access something. You have tools for everything."
	if userCtx != "" {
		planSystem += "\n\nUser context:\n" + userCtx
	}
	planResult, err := ai.Ask(&ai.Prompt{
		System:   planSystem,
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

	// Step 3: Synthesise with user context.
	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	synthSystem := "You are Micro, a personal AI assistant. Today is " + today + ". " +
		"Answer concisely using the tool results and user context below. Use markdown. " +
		"If the user context already contains the answer (e.g. unread mail count), use it directly."
	if userCtx != "" {
		synthSystem += "\n\nUser context:\n" + userCtx
	}
	answer, err := ai.Ask(&ai.Prompt{
		System: synthSystem,
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

	// Check if the user asked to remember something.
	go extractMemory(acc.ID, req.Prompt)

	// Save as a flow so it appears in the agent history at /agent.
	var steps []FlowStep
	for _, tu := range toolsUsed {
		steps = append(steps, FlowStep{Tool: tu.Name})
	}
	flow := &Flow{
		ID:        newFlowID(),
		AccountID: acc.ID,
		Prompt:    req.Prompt,
		Steps:     steps,
		Answer:    answer,
		Status:    "done",
		ParentID:  req.ContextID,
		CreatedAt: time.Now().UTC(),
	}
	saveFlow(flow)

	app.RespondJSON(w, RunResponse{Answer: answer, FlowID: flow.ID, Tools: toolsUsed})
}
