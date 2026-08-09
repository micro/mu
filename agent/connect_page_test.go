package agent

// Clicking an agent should open the agent, not a chat with it.
//
// An external agent is one you built to run somewhere else and call in with a
// token. Its page opened on a chat box — the one thing that agent will never
// use — and the endpoint, the scope and the token it does need were on no page
// at all. The only "connect" link went to /tools, the generic catalogue, from a
// page that already knew which agent you were looking at.

import (
	"html"
	"strings"
	"testing"
)

func TestAnExternalAgentOpensOnHowToReachItAndAHostedOneOnTalkingToIt(t *testing.T) {
	probes(t)
	id := owner(t, "connect-reader")

	out, _, err := CreateAgent(id, "Outsider", External, "runs elsewhere", "", []string{"probealpha"}, true)
	if err != nil {
		t.Fatal(err)
	}
	in, _, err := CreateAgent(id, "Homebody", Hosted, "runs here", "watch things", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if row := agentRow(out, "csrf", "http://localhost"); !strings.Contains(row,
		`class="agent-name" href="/agent/connect?id=`+out.ID) {
		t.Errorf("an external agent's name still opens a chat box it will never use:\n%s", row)
	}
	if row := agentRow(in, "csrf", "http://localhost"); !strings.Contains(row,
		`class="agent-name" href="/agent?id=`+in.ID) {
		t.Errorf("a hosted agent's name no longer opens a conversation with it:\n%s", row)
	}
	// Either kind can be reached from the row without guessing at the URL.
	for _, a := range []*Agent{out, in} {
		if !strings.Contains(agentRow(a, "csrf", "http://localhost"), `/agent/connect?id=`+a.ID+`">Connect`) {
			t.Errorf("%s has no Connect link on /agents", a.Name)
		}
	}
}

// The three things you need to point something at an agent, on one page: what
// it may reach, where to send it, and whether it has a credential.
func TestTheConnectPageCarriesTheScopeTheEndpointAndTheTokenState(t *testing.T) {
	probes(t)
	id := owner(t, "connect-panel")

	a, _, err := CreateAgent(id, "Scoped", External, "reads probes", "", []string{"probealpha"}, false)
	if err != nil {
		t.Fatal(err)
	}

	panel := connectPanel(a, "https://mu.example", "csrf")
	// The scope goes above the token, because it is the thing that makes
	// handing a token out safe.
	if !strings.Contains(panel, "May reach") || strings.Contains(panel, "everything you can reach") {
		t.Errorf("a scoped agent's page does not say what it is confined to:\n%s", panel)
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

// Connect is a tab only when there is an agent to connect to. Mu's own
// assistant has no token, no scope and no address of its own.
func TestTheDefaultAssistantHasNothingToConnectTo(t *testing.T) {
	if tabs := agentTabs("chat", ""); strings.Contains(tabs, "/agent/connect") {
		t.Errorf("the default assistant offers a Connect tab:\n%s", tabs)
	}
	if tabs := agentTabs("chat", "some-agent"); !strings.Contains(tabs, "/agent/connect?id=some-agent") {
		t.Errorf("a named agent has no Connect tab:\n%s", tabs)
	}
}
