package agent

// Reaching one of your agents by name, from anywhere.
//
// This lived in runs_page.go, which was the /agent/runs page — a list of every
// workflow record, prompt by prompt. That page is gone (see the routes it left
// behind, and runTools in inbox/conversation.go, which is the part of it worth
// keeping: what an answer's tools were, beside the answer). These three
// functions were in the same file for no reason beyond both touching an agent.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ── Asking a named agent from anywhere ──────────────────────────
//
// Agents were reachable only through this instance's own web chat. Every other
// caller — MCP, mail, the CLI, a cron job — got the default
// assistant however many you had built, because the only place that resolved a
// named agent was the SSE handler. An agent you cannot invoke from outside is a
// preset on a settings page.

// AskAs turns a name or id into the options a query should run with. An empty
// ref means the default agent, which is not an error — most callers want it.
func AskAs(accountID, ref string) (QueryOpts, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return QueryOpts{}, nil
	}
	a := findAgent(accountID, ref)
	if a == nil {
		return QueryOpts{}, fmt.Errorf("no agent called %q — agent_list names yours", ref)
	}
	m := a.AsMicro()
	// Model too. It was dropped here, so an agent could be given one and then
	// answer on the instance default anyway — the field set, the run ignoring
	// it, and nothing on screen to say which had happened.
	return QueryOpts{System: m.SystemPrompt, Tools: m.Tools, Model: m.Model}, nil
}

// findAgent resolves by id first, then by name, case-insensitively. By name
// because somebody writing a call by hand knows what they named it and does not
// know its uuid; by id first because an id is unambiguous and a name need not be.
func findAgent(accountID, ref string) *Agent {
	if a := For(accountID, ref); a != nil {
		return a
	}
	want := strings.ToLower(ref)
	for _, a := range Agents(accountID) {
		if strings.ToLower(a.Name) == want {
			return a
		}
	}
	// A published agent somebody else owns used to resolve here too. The
	// directory it read from had no page and no callers; see roster.go.
	return nil
}

// ListForCaller names an account's agents as JSON, for agent_list.
func ListForCaller(accountID string) (string, error) {
	type lite struct {
		Name  string   `json:"name"`
		About string   `json:"about,omitempty"`
		Tools []string `json:"tools,omitempty"`
		Email string   `json:"email,omitempty"`
	}
	out := []lite{{Name: "Micro", About: "The default agent. Every tool you can reach."}}
	for _, a := range Agents(accountID) {
		out = append(out, lite{
			Name:  a.Name,
			About: firstLine(a.Description, a.Prompt),
			Tools: a.Services,
			Email: a.Address(),
		})
	}
	b, err := json.Marshal(map[string]any{"agents": out})
	return string(b), err
}
