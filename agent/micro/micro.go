// Package micro implements the multi-agent system. Each domain (news,
// markets, mail, etc.) gets a specialised agent with its own system
// prompt, tool set, and memory scope. A lightweight router picks the
// right agent for each query. The default "micro" agent has all tools
// and serves as the catch-all — identical to the pre-multi-agent behaviour.
package micro

// Agent defines a specialised micro-agent. Built-in agents have an empty
// OwnerAccountID; user-defined agents are owned by the account that created them.
type Agent struct {
	ID             string   // "markets", "news", or a user agent id like "u_xxxx"
	Name           string   // Human-friendly: "Markets Agent"
	Description    string   // One-line for the router
	SystemPrompt   string   // Personality and instructions
	Tools          []string // Allowed MCP tool names
	MemoryScope    string   // Memory namespace prefix (e.g. "markets")
	OwnerAccountID string   `json:",omitempty"` // empty = built-in; else the creator
	ForkedFrom     string   `json:",omitempty"` // id this was forked from, if any
}

// Registry maps agent IDs to their (built-in) definitions.
var Registry = map[string]*Agent{}

// UserAgentResolver, when set, resolves an account's custom agent by ID. It lets
// the executor run user-defined agents without the micro package owning storage.
var UserAgentResolver func(accountID, id string) *Agent

// Register adds a built-in agent to the registry.
func Register(a *Agent) {
	Registry[a.ID] = a
}

// Get returns a built-in agent by ID, or nil.
func Get(id string) *Agent {
	return Registry[id]
}

// resolve finds a built-in agent, or the account's custom agent, by ID.
func resolve(accountID, id string) *Agent {
	if a := Get(id); a != nil {
		return a
	}
	if UserAgentResolver != nil {
		if a := UserAgentResolver(accountID, id); a != nil {
			return a
		}
	}
	return nil
}

// All returns all registered agents (excluding "micro" fallback).
func All() []*Agent {
	var out []*Agent
	for _, a := range Registry {
		if a.ID != "micro" {
			out = append(out, a)
		}
	}
	return out
}
