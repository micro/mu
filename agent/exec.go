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
)

// looksLikeExecRequest checks if the prompt is asking to build, create, or make something.
func looksLikeExecRequest(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, keyword := range []string{
		"build me", "build a", "build an", "create a", "create an",
		"make me", "make a", "make an", "write a", "write an",
		"build app", "create app", "make app",
		"calculator", "timer", "converter", "generator", "tracker",
		"editor", "viewer", "dashboard", "tool",
	} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// runWorkspaceFlow handles build/exec requests within the agent SSE stream.
func runWorkspaceFlow(w http.ResponseWriter, r *http.Request, flow *Flow, prompt string, model Model) {
	sseSend := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	sseSend(map[string]any{"type": "thinking", "message": "Planning…"})

	// Plan with exec capability
	planSystem := `You are an AI agent on a browser-based app platform.

STEP TYPES:
1. {"type":"tool","name":"TOOL","args":{}} — fetch data server-side
2. {"type":"exec","html":"..."} — render full HTML app in browser preview
3. {"type":"exec","code":"..."} — run JS in browser
4. {"type":"respond","message":"markdown text"} — text answer

TOOLS (for fetching data):
` + agentToolsDesc + `

RULES:
- Building apps → use EXEC with a complete HTML document
- Data questions → use TOOLS then RESPOND
- Simple questions → just RESPOND
- NEVER use exec for fetching data

Output ONLY a JSON array.`

	planResult, err := ai.Ask(&ai.Prompt{
		System:   planSystem,
		Question: prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "agent-exec-plan",
	})
	if err != nil {
		sseSend(map[string]any{"type": "error", "message": err.Error()})
		sseSend(map[string]any{"type": "done"})
		return
	}

	type step struct {
		Type    string         `json:"type"`
		Code    string         `json:"code,omitempty"`
		HTML    string         `json:"html,omitempty"`
		Name    string         `json:"name,omitempty"`
		Args    map[string]any `json:"args,omitempty"`
		Message string         `json:"message,omitempty"`
	}
	var steps []step
	stepsJSON := extractJSONArray(planResult)
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		// Treat as text response
		rendered := app.RenderString(planResult)
		sseSend(map[string]any{"type": "response", "html": rendered, "flow_id": flow.ID})
		updateFlow(flow.ID, func(f *Flow) { f.Status = "done"; f.Answer = planResult; f.HTML = rendered })
		sseSend(map[string]any{"type": "done"})
		return
	}

	// Execute steps
	var toolResults []string
	responded := false

	for _, s := range steps {
		switch s.Type {
		case "exec":
			code := stripCodeFences(s.Code)
			sseSend(map[string]any{"type": "exec", "code": code, "html": s.HTML})
			// Wait for browser feedback
			fb := waitForFeedback(flow.ID, 15*time.Second)
			if fb != nil && !fb.OK && fb.Error != "" {
				sseSend(map[string]any{"type": "thinking", "message": "Fixing: " + fb.Error})
				fixResult, fixErr := ai.Ask(&ai.Prompt{
					System:   "Fix this error. Output ONLY the corrected code or HTML. No markdown fences.",
					Question: fmt.Sprintf("Error: %s\nCode:\n%s", fb.Error, s.Code+s.HTML),
					Priority: ai.PriorityHigh,
					Caller:   "agent-exec-fix",
				})
				if fixErr == nil {
					sseSend(map[string]any{"type": "exec", "code": stripCodeFences(fixResult), "html": ""})
					waitForFeedback(flow.ID, 15*time.Second)
				}
			}

		case "tool":
			sseSend(map[string]any{"type": "tool_start", "name": s.Name, "message": toolLabel(s.Name)})
			text, isErr, execErr := api.ExecuteTool(r, s.Name, s.Args)
			if execErr != nil || isErr {
				sseSend(map[string]any{"type": "tool_done", "name": s.Name, "message": s.Name + " — failed"})
				continue
			}
			if len(text) > 8000 {
				text = text[:8000]
			}
			toolResults = append(toolResults, fmt.Sprintf("### %s\n%s", s.Name, formatToolResult(s.Name, text, s.Args)))
			sseSend(map[string]any{"type": "tool_done", "name": s.Name, "message": toolLabel(s.Name) + " — done"})

		case "respond":
			responded = true
			rendered := app.RenderString(s.Message)
			sseSend(map[string]any{"type": "response", "html": rendered, "flow_id": flow.ID})
			updateFlow(flow.ID, func(f *Flow) { f.Status = "done"; f.Answer = s.Message; f.HTML = rendered })
		}
	}

	// Synthesise if tools ran but no response
	if len(toolResults) > 0 && !responded {
		sseSend(map[string]any{"type": "thinking", "message": "Composing answer…"})
		answer, err := ai.Ask(&ai.Prompt{
			System:   "Summarise the results. Use markdown.",
			Rag:      toolResults,
			Question: prompt,
			Priority: ai.PriorityHigh,
			Caller:   "agent-exec-synth",
		})
		if err == nil {
			answer = app.StripLatexDollars(answer)
			rendered := app.RenderString(answer)
			sseSend(map[string]any{"type": "response", "html": rendered, "flow_id": flow.ID})
			updateFlow(flow.ID, func(f *Flow) { f.Status = "done"; f.Answer = answer; f.HTML = rendered })
		}
	}

	if !responded && len(toolResults) == 0 {
		updateFlow(flow.ID, func(f *Flow) { f.Status = "done"; f.Answer = "App built" })
	}

	sseSend(map[string]any{"type": "done"})
}
