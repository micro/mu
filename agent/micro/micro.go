// Package micro implements the multi-agent system. Each domain (news,
// markets, mail, etc.) gets a specialised agent with its own system
// prompt, tool set, and memory scope. A lightweight router picks the
// right agent for each query. The default "micro" agent has all tools
// and serves as the catch-all — identical to the pre-multi-agent behaviour.
package micro

// Agent defines a specialised micro-agent.
type Agent struct {
	ID           string   // "markets", "news", "mail", etc.
	Name         string   // Human-friendly: "Markets Agent"
	Description  string   // One-line for the router
	SystemPrompt string   // Personality and instructions
	Tools        []string // Allowed MCP tool names
	MemoryScope  string   // Memory namespace prefix (e.g. "markets")
}

// Registry maps agent IDs to their definitions.
var Registry = map[string]*Agent{}

// Register adds an agent to the registry.
func Register(a *Agent) {
	Registry[a.ID] = a
}

// Get returns an agent by ID, or nil.
func Get(id string) *Agent {
	return Registry[id]
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
