package agent

// An agent that runs elsewhere cannot answer here.
//
// "Answering as" listed every agent, external ones included, and picking one
// did something quietly different from what it said: AskAs returns that
// agent's system prompt and tool scope, and Mu's own model answers with them.
// An external agent usually has no prompt — the whole point is that something
// outside supplies the model — so choosing it got you the default assistant
// with a narrower toolbox and no sign that was what happened.
//
// Nothing here can hand a question to something running elsewhere. A2A is
// implemented in the inbound direction only: external agents discover and call
// Mu, not the reverse. Until that changes, the honest thing is not to offer it.

import (
	"os"
	"strings"
	"testing"
)

func readAgentSource(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

func TestTheChatPickerLeavesOutAgentsThatRunElsewhere(t *testing.T) {
	src := readAgentSource(t, "../internal/app/chat.go")
	if !strings.Contains(src, `a.kind!=='external'`) {
		t.Error("the chat picker offers external agents, which answer as the default " +
			"assistant wearing their scope rather than as themselves")
	}

	// The filter is worthless if the field never arrives. Asserted on the field
	// and the value that fills it, not on the whole struct literal — that was
	// spelled "m.Tools, a.Kind}", so adding any field after Kind failed a test
	// about something else entirely.
	data := readAgentSource(t, "agents.go")
	if !strings.Contains(data, `json:"kind,omitempty"`) || !strings.Contains(data, "a.Kind") {
		t.Error("/agents/data no longer reports where an agent runs, so nothing " +
			"downstream can tell the two kinds apart")
	}
}

// Clicking one opens how to reach it, the same as /agents does. One door, one
// behaviour — the rail used to send you to a chat box instead.
func TestTheRailOpensAnExternalAgentOnConnect(t *testing.T) {
	src := readAgentSource(t, "agents.go")
	if !strings.Contains(src, "function muAgentOpen(id,external)") {
		t.Fatal("the rail has no way to tell the two kinds apart when opening one")
	}
	if !strings.Contains(src, `window.location='/agent/connect?id='+encodeURIComponent(id)`) {
		t.Error("an external agent in the rail still opens a chat with something that cannot chat")
	}
	if !strings.Contains(src, `onclick="muAgentOpen(`) {
		t.Error("the rail rows are not going through muAgentOpen, so the distinction is not applied")
	}
}
