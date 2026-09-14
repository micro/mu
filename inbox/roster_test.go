package inbox

// The agent hooks, as the server fills them in.

import (
	"strings"
	"testing"
)

// withRoster wires both agent hooks for a test: the names and the address tags,
// which is the pair internal/server hands over.
//
// Together, because they answer the same question and a test that sets one and
// not the other is testing a state the product never has — which is how these
// tests went on passing while a box's name and its address were derived from
// two different rules. See inbox.Agents.
func withRoster(t *testing.T, owner string, agents ...Agent) {
	t.Helper()
	Agents = func(o string) []Agent {
		if o != owner {
			return nil
		}
		return agents
	}
	AgentName = func(o, id string) string {
		if o != owner {
			return ""
		}
		for _, a := range agents {
			if a.ID == id {
				return a.Name
			}
		}
		return ""
	}
	t.Cleanup(func() { Agents, AgentName = nil, nil })
}

func TestLegacyAgentLinkFiltersOwnedHistory(t *testing.T) {
	const who = "history_agent"
	withRoster(t, who, Agent{ID: "one", Name: "Research", Tag: "research"}, Agent{ID: "two", Name: "Research", Tag: "research2"})
	arrived(t, who, "mail", "first", "one", "a@example.com", "First research message")
	arrived(t, who, "mail", "second", "two", "a@example.com", "Second research message")
	body := listBody(t, "/inbox/research2", who, "research2")
	if !strings.Contains(body, "Second research message") || strings.Contains(body, "First research message") {
		t.Fatal("legacy link lost agent scoping")
	}
}
