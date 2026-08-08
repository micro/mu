package agent

// Switching agent switches the page, not just the label.
//
// The agents list in the rail set window.muActiveAgent, moved the highlight and
// rewrote the chip, and did nothing else. The conversation in the middle and
// the rail of past conversations beside it are both rendered by the server for
// one agent, so picking a second agent left the first one's history on screen
// under a chip naming the second — and a brand-new agent, whose rail should be
// empty, showed the conversations of whichever agent you had just left.

import (
	"strings"
	"testing"
)

func TestPickingAnAgentNavigatesRatherThanRelabelling(t *testing.T) {
	panel := renderAgentsPanel()

	if !strings.Contains(panel, "window.location='/agent'") &&
		!strings.Contains(panel, "window.location=to") {
		t.Error("picking an agent still does not navigate, so the rail and the conversation " +
			"beside it keep showing the agent you just left")
	}
	if strings.Contains(panel, "function muAgentPick(id){window.muActiveAgent=id") {
		t.Error("muAgentPick is back to setting a variable and moving a highlight")
	}
	// The id belongs in the URL, the same way /agents links to an agent, so a
	// reload keeps the agent instead of falling back to the default.
	if !strings.Contains(panel, "'/agent?id='+encodeURIComponent(id)") {
		t.Error("the chosen agent does not reach the URL")
	}
}

// An agent with no conversations has an empty rail, and says so. The rail is
// filtered by the agent the page is for, so this is only true if the page is
// actually re-rendered for the agent that was picked.
func TestARailForOneAgentIsEmptyUntilThatAgentHasBeenUsed(t *testing.T) {
	acc := owner(t, "rail-reader")

	rail := renderSessionsRail(acc, "", "agent-with-no-history")
	if !strings.Contains(rail, "No conversations with this agent yet.") {
		t.Errorf("a fresh agent's rail does not read as empty:\n%s", rail)
	}
	// New chat keeps the agent, rather than rewriting the address bar back to
	// the default and quietly widening the rail to the whole account.
	if !strings.Contains(rail, "/agent?id=agent-with-no-history") {
		t.Errorf("+ New chat drops the agent out of the URL:\n%s", rail)
	}

	// The account-wide rail is a different sentence, because it means
	// something different: nothing has been asked at all.
	if all := renderSessionsRail(acc, "", ""); !strings.Contains(all, "No conversations yet.") {
		t.Errorf("the unfiltered rail lost its empty state:\n%s", all)
	}
}
