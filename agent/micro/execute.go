package micro

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/memory"
)

// UserContextFunc is set by main.go. Returns user context string.
var UserContextFunc func(accountID string) string

// QueryOpts matches agent.QueryOpts for compatibility.
type QueryOpts struct {
	History []struct {
		Role string
		Text string
	}
	Public    bool
	AccountID string
}

// Execute runs a specialised agent: plan → execute tools → synthesise.
func (a *Agent) Execute(accountID, prompt string, public bool) (string, error) {
	app.Log("micro", "Agent %s handling: %.80s", a.ID, prompt)

	// Build tool list description for planning
	toolsDesc := a.buildToolsDesc()

	// User context (skip for public/guest queries)
	userCtx := ""
	if !public && UserContextFunc != nil {
		userCtx = UserContextFunc(accountID)
	}

	// Agent-scoped memory
	agentMemory := ""
	if a.MemoryScope != "" && !public {
		agentMemory = memory.ForScopedContext(accountID, a.MemoryScope)
	}

	// Plan
	planSystem := "You are the " + a.Name + ". Given a user question, output ONLY a JSON array of tool calls.\n\n" +
		toolsDesc +
		"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tool calls. Output [] if no tools needed."
	if userCtx != "" {
		planSystem += "\n\nUser context:\n" + userCtx
	}
	if agentMemory != "" {
		planSystem += "\n\nYour memory:\n" + agentMemory
	}

	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	var toolCalls []toolCall

	planResult, err := ai.Ask(&ai.Prompt{
		System:   planSystem,
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityHigh,
		Caller:   "micro-" + a.ID + "-plan",
	})
	if err == nil {
		planJSON := extractJSONArray(planResult)
		json.Unmarshal([]byte(planJSON), &toolCalls)
	}

	// Execute tools
	type toolResult struct {
		Name   string
		Result string
	}
	var results []toolResult

	for _, tc := range toolCalls {
		if tc.Tool == "" {
			continue
		}
		if !a.hasTool(tc.Tool) {
			continue
		}
		text, isErr, execErr := api.ExecuteToolAs(accountID, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			continue
		}
		if len(text) > 8000 {
			text = text[:8000] + "…"
		}
		results = append(results, toolResult{Name: tc.Tool, Result: text})
	}

	// Synthesise
	var ragParts []string
	for _, res := range results {
		ragParts = append(ragParts, fmt.Sprintf("### %s\n%s", res.Name, res.Result))
	}

	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	synthSystem := a.SystemPrompt + "\n\nToday is " + today + "."
	if len(results) > 0 {
		synthSystem += "\n\nUse the tool results below to answer. Be concise."
	}
	if !public && userCtx != "" {
		synthSystem += "\n\nUser context:\n" + userCtx
	}

	answer, err := ai.Ask(&ai.Prompt{
		System:   synthSystem,
		Rag:      ragParts,
		Question: prompt,
		Priority: ai.PriorityHigh,
		Caller:   "micro-" + a.ID + "-synth",
	})
	if err != nil {
		return "", err
	}

	return app.StripLatexDollars(answer), nil
}

func (a *Agent) buildToolsDesc() string {
	// All tools descriptions from the MCP registry
	allDescs := api.ToolDescriptions()
	if a.Tools == nil {
		return allDescs // "micro" agent gets everything
	}

	allowed := map[string]bool{}
	for _, t := range a.Tools {
		allowed[t] = true
	}

	var sb strings.Builder
	sb.WriteString("Available tools (use exact name):\n")
	for _, line := range strings.Split(allDescs, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		// Extract tool name from "- toolname: description"
		rest := line[2:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			continue
		}
		name := strings.TrimSpace(rest[:colonIdx])
		if allowed[name] {
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func (a *Agent) hasTool(name string) bool {
	if a.Tools == nil {
		return true // "micro" agent has all tools
	}
	for _, t := range a.Tools {
		if t == name {
			return true
		}
	}
	return false
}

func extractJSONArray(text string) string {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start == -1 || end == -1 || end <= start {
		return "[]"
	}
	return text[start : end+1]
}

// Orchestrate runs one or more agents and merges their answers.
func Orchestrate(accountID, prompt string, agentIDs []string, public bool) (string, error) {
	if len(agentIDs) == 1 {
		a := Get(agentIDs[0])
		if a == nil {
			a = Get("micro")
		}
		return a.Execute(accountID, prompt, public)
	}

	// Run agents in parallel
	type result struct {
		AgentID string
		Answer  string
		Err     error
	}
	ch := make(chan result, len(agentIDs))
	for _, id := range agentIDs {
		go func(agentID string) {
			a := Get(agentID)
			if a == nil {
				a = Get("micro")
			}
			ans, err := a.Execute(accountID, prompt, public)
			ch <- result{agentID, ans, err}
		}(id)
	}

	var parts []string
	for range agentIDs {
		r := <-ch
		if r.Err == nil && strings.TrimSpace(r.Answer) != "" {
			name := r.AgentID
			if a := Get(r.AgentID); a != nil {
				name = a.Name
			}
			parts = append(parts, fmt.Sprintf("### %s\n%s", name, r.Answer))
		}
	}

	if len(parts) == 0 {
		return Get("micro").Execute(accountID, prompt, public)
	}
	if len(parts) == 1 {
		// Strip the header if only one agent responded
		lines := strings.SplitN(parts[0], "\n", 2)
		if len(lines) > 1 {
			return lines[1], nil
		}
		return parts[0], nil
	}

	// Merge multiple agent responses
	today := time.Now().UTC().Format("Monday, 2 January 2006 (UTC)")
	merged, err := ai.Ask(&ai.Prompt{
		System:   "You are Micro, a personal AI. Today is " + today + ". Multiple specialists provided answers below. Combine them into one coherent response. Highlight connections between domains. Be concise.",
		Rag:      parts,
		Question: prompt,
		Priority: ai.PriorityHigh,
		Caller:   "micro-merge",
	})
	if err != nil {
		return strings.Join(parts, "\n\n"), nil
	}
	return app.StripLatexDollars(merged), nil
}
