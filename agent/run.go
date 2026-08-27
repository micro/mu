package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
)

// extractMemory checks if the user's prompt contains something to
// remember (preferences, facts about themselves, interests). Runs
// async after the response so it doesn't slow down the answer.
//
// scope is the agent the conversation was with, and it is the half that was
// missing. Every one of this instance's agents declares a MemoryScope —
// "weather", "markets", "faith" — read by notes.ForScopedContext to give each
// one its own memory on top of the shared pool. Nothing ever wrote into it. So
// eleven agents had eleven namespaces, all of them empty, and every one of them
// saw the same unscoped notes: separate agents with identical memories, which
// is one agent with eleven names.
//
// What goes where is the model's call and the prompt says how to make it: a
// fact about the person is theirs and belongs to everybody, a fact that only
// matters to this specialty is scoped. "I live in London" is not the weather
// agent's — the markets agent needs it for the exchange, the places agent needs
// it to find a cafe. "I only care about the 5am forecast" is nobody else's
// business, and putting it in the shared pool is how every agent's context
// silently fills with the last ten conversations you had with a different one.
func extractMemory(accountID, prompt, scope string) {
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

	system := `Extract any personal preference or fact the user is sharing about themselves.
Output ONLY valid JSON: {"key":"short label","value":"what to remember"}
If the message does NOT contain a personal preference or fact, output: {}
Examples:
"Remember I like Bitcoin" → {"key":"interest","value":"likes Bitcoin"}
"I live in London" → {"key":"location","value":"London"}
"What's the weather?" → {}`
	if scope != "" {
		system += `

This was said to the ` + scope + ` specialist. Add "scoped":true when the fact is
only of use to that specialist and would be noise to any other, and leave it out
when it is a fact about the person that anything should know.
"I live in London" → {"key":"location","value":"London"}
"Only show me the 5am forecast" → {"key":"forecast","value":"wants the 5am one","scoped":true}`
	}

	result, err := ai.Ask(&ai.Prompt{
		System:   system,
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Caller:   "memory-extract",
	})
	if err != nil || result == "" {
		return
	}
	var extracted struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Scoped bool   `json:"scoped"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &extracted); err != nil {
		return
	}
	if extracted.Key == "" || extracted.Value == "" {
		return
	}
	title := extracted.Key
	if extracted.Scoped && scope != "" {
		// The prefix is the namespace, and it is a colon rather than a field
		// because notes are a title and a body and nothing else — see
		// internal/notes. ForScopedContext reads it back and strips it.
		title = scope + ":" + extracted.Key
	}
	notes.Add(accountID, title, extracted.Value)
	app.Log("memory", "Saved for %s: %s = %s", accountID, title, extracted.Value)
}

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

	// No scope: /agent/run names no agent, so there is no specialist for a
	// fact to belong to and it goes in the shared pool, which is where a fact
	// with no owner belongs.
	go extractMemory(acc.ID, req.Prompt, "")
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
