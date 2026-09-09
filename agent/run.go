package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mu/agent/micro"
	"mu/internal/app"
	"mu/internal/auth"
)

// scopeOf is the memory namespace of the agent that answered, empty for the
// general one and for an agent somebody made themselves.
//
// A user's own agent has no scope on purpose. They made one thing and they get
// one memory; namespacing it would split their own facts across agents they did
// not know were separate, and there is no registry entry to declare a scope in.
func scopeOf(agentID string) string {
	if agentID == "" {
		return ""
	}
	if a := micro.Get(agentID); a != nil {
		return a.MemoryScope
	}
	return ""
}

// UserContextFunc is set by main.go to provide personalised context
// for the agent's responses. Returns a string with the user's current
// state (unread mail, market prices, etc.) that gets injected into the
// synthesis prompt.
var UserContextFunc func(accountID string) string

// CardContextFunc returns the reader's home cards as text, when they ask for
// them. Set by main.go from the home package — a hook because home renders the
// cards and this package must not import it.
var CardContextFunc func(accountID string) string

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

	// An account, or nothing.
	//
	// A signed-out visitor used to get three agent runs a day against a
	// per-IP counter and an instance-wide ceiling — a demonstration, so the
	// landing could show the tools being used rather than described. The
	// landing has no chat box on it any more, and every account now gets a
	// daily allowance of its own (internal/quota/allowance.go), so what was a
	// second free tier with different rules is just the first one, reached by
	// signing up.
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.WriteHeader(401)
		app.RespondJSON(w, RunResponse{Error: "Sign in to ask the agent."})
		return
	}

	// The same agent every other door reaches.
	//
	// This was a third copy of the plan/execute/synthesize pipeline — its own
	// tool catalogue, its own two system prompts, its own dedupe — and being a
	// copy is what made it wrong rather than merely redundant. It named no
	// agent, so a user-defined agent could not be reached through it; it did no
	// routing, so it always answered as the generalist; and its catalogue was a
	// hand-written list that had to be edited whenever a service was added,
	// which is how a door onto "every tool" came to offer a different set of
	// tools from the one next to it.
	//
	// OnStep is what fills Tools in the response, and it now comes from the
	// tool wrapper rather than from a loop here — so the report is of what the
	// model actually called.
	var toolsUsed []ToolUsed
	var steps []FlowStep
	answer, err := QueryWithOpts(acc.ID, req.Prompt, QueryOpts{
		OnStep: func(s Step) {
			status := "ok"
			if !s.OK {
				status = "error"
			}
			toolsUsed = append(toolsUsed, ToolUsed{Name: s.Tool, Status: status})
			steps = append(steps, FlowStep{Tool: s.Tool, Args: s.Args})
		},
	})
	if err != nil {
		app.RespondJSON(w, RunResponse{Error: err.Error(), Tools: toolsUsed})
		return
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
