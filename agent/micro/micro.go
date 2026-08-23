// Package micro is what an agent is made of, and the agents this instance
// provides.
//
// An agent here is four things: a name, an instruction, the tools it may reach,
// and a memory scope. That is the whole type, and it is the same type whether
// the agent is this instance's or one somebody made — which is what lets the
// roster in agent/ hand one over to be run without a second shape to convert
// between.
//
// It used to be a multi-agent system in the fuller sense: eleven agents, a
// keyword router that picked one out of the wording, an LLM call for the
// prompts the keywords missed, and an orchestrator that ran two or three in
// parallel and merged their answers through a second plan → execute →
// synthesise loop of its own. All of it is deleted. The router and the
// orchestrator existed to choose between the ten specialists; the specialists
// were the same tools behind ten names, and choosing between them was work the
// caller never asked for and could not check.
//
// What is left is the registry, the type, and addressing one by name.
package micro

// Agent defines an agent: a name, an instruction, a tool scope, a memory scope.
// This instance's own have an empty OwnerAccountID; an account's agents are
// owned by whoever made them.
type Agent struct {
	ID           string   // "micro", or a user agent id like "u_xxxx"
	Name         string   // Human-friendly: "Micro"
	Description  string   // One line: what it is for
	SystemPrompt string   // Personality and instructions
	Tools        []string // Allowed MCP tool names; nil means every one
	MemoryScope  string   // Memory namespace prefix
	// Examples are things worth asking this agent, in its own words.
	//
	// Every chat box on the site read "Try: give me a morning brief",
	// including the one on the Weather agent's page — which is a suggestion
	// for a different agent, offered by the page you opened to get away from
	// the general one. A specialist whose prompt cannot say what it is for is
	// indistinguishable from the default with a different name on it.
	//
	// The first is the placeholder; all of them are the pills under the box.
	Examples       []string
	OwnerAccountID string `json:",omitempty"` // empty = built-in; else the creator
	ForkedFrom     string `json:",omitempty"` // id this was forked from, if any
}

// Registry maps agent IDs to their (built-in) definitions.
var Registry = map[string]*Agent{}

// Register adds a built-in agent to the registry.
func Register(a *Agent) {
	Registry[a.ID] = a
}

// Get returns a built-in agent by ID, or nil.
func Get(id string) *Agent {
	return Registry[id]
}
