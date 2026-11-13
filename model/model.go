// Package model defines the composition used by agents and generated writing:
// an LLM, a foundation, instructions, knowledge and services. It performs no
// inference, retrieval or tool execution and holds no account state.
package model

import "strings"

// Model describes a composition, independently of a particular agent or run.
// Runtime credentials, permissions, conversation history and personal memory
// are supplied by the caller, never stored in this shared definition.
type Model struct {
	LLM          LLM
	Foundation   string
	Instructions string
	Knowledge    []Source
	Services     []string // Service references, not authorization grants.
}

// LLM selects the trained model. Empty fields retain the runtime's configured
// provider and model selection; this package does not resolve credentials.
type LLM struct {
	Provider string
	ID       string
}

// Source identifies reference material accessed through an existing service.
// URL describes its origin, not a URL to fetch automatically. Retrieved text
// is evidence and must retain its provenance; it is not a system instruction.
type Source struct {
	Name    string
	URL     string
	Service string
}

// Default returns an independent definition of Mu's shared foundation and
// reference sources. Listing a source neither fetches it nor enables a tool.
// The existing runtime continues to resolve LLMs and enforce service access.
func Default() Model {
	return Model{
		Foundation: Foundation,
		Knowledge: []Source{
			{Name: "Reminder", URL: "https://reminder.dev", Service: "islam"},
			{Name: "Aslam", URL: "https://aslam.org", Service: "islam"},
		},
	}
}

// SystemPrompt layers the foundation, model instructions and instructions for
// this use. Knowledge contents and personal context must be supplied separately.
// Already composed instructions retain a single leading foundation.
func (m Model) SystemPrompt(instructions string) string {
	if m.Foundation != "" && m.Instructions == "" && strings.HasPrefix(instructions, m.Foundation) {
		return instructions
	}
	parts := []string{}
	if m.Foundation != "" {
		parts = append(parts, m.Foundation)
	}
	for _, text := range []string{m.Instructions, instructions} {
		if m.Foundation != "" && strings.HasPrefix(text, m.Foundation) {
			text = strings.TrimPrefix(text, m.Foundation)
			text = strings.TrimLeft(text, "\r\n")
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
