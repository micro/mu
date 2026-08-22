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

// The agent you have never written to is the one whose address you need.
//
// The switcher was derived from what had arrived, so an agent with no mail had
// no box — and once the box began carrying that agent's address, no box meant
// no way to find out where to write. A box with nothing in it says so.
func TestAnAgentWithNoMailStillHasABox(t *testing.T) {
	const who = "inbox-unwritten"
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	withRoster(t, who, Agent{ID: "a1", Name: "Research", Tag: "research"})

	all := listBody(t, "/inbox", who, "")
	if !strings.Contains(all, `href="/inbox/research"`) {
		t.Errorf("an agent with no mail has no box:\n%s", all)
	}

	one := listBody(t, "/inbox/research", who, "research")
	if !strings.Contains(one, "research@micro.mu") {
		t.Errorf("the empty box does not say where to write:\n%s", one)
	}
}

// A box is the agent's address tag, not a slug of its name.
//
// They were two rules over one thing. A tag is cleaned, cut at 24 characters
// and made unique with a numeric suffix — two agents named alike are research
// and research2 — where the slug kept the whole name, stripped a different set
// of characters, and knew nothing about the other agent. So the address shown
// above a box was somebody else's, or nobody's.
func TestABoxIsTheAddressTag(t *testing.T) {
	const who = "inbox-tags"
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	withRoster(t, who,
		Agent{ID: "a1", Name: "Research", Tag: "research"},
		// Same name, so the same slug — and a tag of its own.
		Agent{ID: "a2", Name: "Research", Tag: "research2"},
		// Longer than a tag may be, so the two cannot agree by accident.
		Agent{ID: "a3", Name: "A very long agent name indeed", Tag: "averylongagentnameindeed"})

	all := listBody(t, "/inbox", who, "")
	for _, tag := range []string{"research", "research2", "averylongagentnameindeed"} {
		if !strings.Contains(all, `href="/inbox/`+tag+`"`) {
			t.Errorf("no box for the tag %q:\n%s", tag, all)
		}
	}

	// And each one names its own address rather than the first agent's.
	for _, tc := range []struct{ box, want string }{
		{"research2", who + "+research2@micro.mu"},
		{"averylongagentnameindeed", who + "+averylongagentnameindeed@micro.mu"},
	} {
		body := listBody(t, "/inbox/"+tc.box, who, tc.box)
		if !strings.Contains(body, tc.want) {
			t.Errorf("the %s box does not name %s:\n%s", tc.box, tc.want, body)
		}
	}
}
