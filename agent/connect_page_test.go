package agent

// How to reach an agent, on a page about that agent.
//
// The endpoint, the scope and the token were on no page at all: the only
// "connect" link went to /tools, the generic catalogue, from a page that
// already knew which agent you were looking at.

import (
	"html"
	"strings"
	"testing"
)

func TestAnAgentsNameOpensAConversationWithIt(t *testing.T) {
	probes(t)
	id := owner(t, "connect_reader")

	// There were two kinds, and an agent declared "external" opened on the
	// Connect page instead of a chat, because there was nothing here to talk
	// to. Every agent runs here now, so every name opens on the conversation.
	a, _, err := CreateAgent(id, "Homebody", Hosted, "runs here", "watch things", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// By name, not by id in a query string. An agent is a place — /agent/<name>,
	// the way /mail and /news are — and the roster is the one thing building
	// those links, so it and the redirect cannot disagree about where an agent
	// lives. See slug.go.
	row := agentRow(a, "csrf", "http://localhost")
	if !strings.Contains(row, `class="agent-name" href="`+Path(id, a.ID)+`"`) {
		t.Errorf("an agent's name no longer opens a conversation with it:\n%s", row)
	}
	if strings.Contains(row, "/agent/connect?id=") && !strings.Contains(row, ">Connect<") {
		t.Errorf("the name still points at the Connect page:\n%s", row)
	}
}

// The three things you need to point something at an agent, on one page: what
// it may reach, where to send it, and whether it has a credential.
func TestTheConnectPageCarriesTheScopeTheEndpointAndTheTokenState(t *testing.T) {
	probes(t)
	id := owner(t, "connect_panel")

	a, _, err := CreateAgent(id, "Scoped", Hosted, "reads probes", "", []string{"probealpha"}, false)
	if err != nil {
		t.Fatal(err)
	}

	panel := connectPanel(a, "https://mu.example", "csrf")
	// The scope goes above the token, because it is the thing that makes
	// handing a token out safe. Labelled "Tools", which is what it is a list
	// of — "May reach" named the same thing in words that could equally have
	// meant the mail address two rows down.
	if !strings.Contains(panel, `conn-k">Tools<`) || !strings.Contains(panel, "Probealpha") {
		t.Errorf("a scoped agent's page does not say what it is confined to:\n%s", panel)
	}
	// And a confined agent must not read as an unconfined one.
	if strings.Contains(panel, "Everything") {
		t.Errorf("a scoped agent's page says it reaches everything:\n%s", panel)
	}
	if !strings.Contains(panel, a.Endpoint("https://mu.example")) {
		t.Errorf("the endpoint is missing, so the scope is not reachable:\n%s", panel)
	}
	// No token was issued, so the page says so and offers one rather than
	// leaving the reader to find /agents.
	if !strings.Contains(panel, "None yet") || !strings.Contains(panel, `name="action" value="token"`) {
		t.Errorf("an agent with no token cannot be given one from here:\n%s", panel)
	}
	// The client config block is this agent's, not the catalogue's example.
	// It is escaped for display, so match what a reader would copy out of it.
	if !strings.Contains(panel, html.EscapeString(`"url": "`+a.Endpoint("https://mu.example")+`"`)) {
		t.Errorf("the client config does not point at this agent:\n%s", panel)
	}
}

// The default agent has a Connect page too.
//
// It used not to, on the reasoning that Micro has no token, no scope and no
// address of its own. Two of those were wrong: it has an address — agent@, the
// one with nothing to remember — and it reaches every tool, which is a scope and
// the widest one going. Only the token is genuinely absent, and "it uses your
// account's" is a thing to say rather than a reason to withhold the page.
//
// The cost of withholding it was not just a missing page: the tab strip changed
// shape depending on which agent was selected, and the agent nearly everybody
// uses was the one with no answer to "how do I reach this".
func TestTheDefaultAgentSaysHowToReachIt(t *testing.T) {
	panel := defaultPanel("https://mu.example")
	for _, want := range []string{
		`conn-k">Tools<`,                        // what it reaches, and the label for it
		"Everything",                            // its scope, which is the widest one
		"https://mu.example/mcp",                // where something points at it
		`href="/token"`,                         // the credential it actually uses
		html.EscapeString(`"url": "https://mu`), // a config block to copy
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("the default agent's page does not say %q:\n%s", want, panel)
		}
	}
	// It is not an agent you can be issued a token for or delete: it is this
	// account, and offering either would be offering something that cannot happen.
	if strings.Contains(panel, `name="action" value="token"`) {
		t.Error("the default agent offers to issue itself a token")
	}
}
