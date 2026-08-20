package agent

// Every agent on the rail is one you can talk to.
//
// There were two kinds. An agent declared "external" ran in Claude or Cursor
// and called in with a token, so nothing here could hand it a question:
// "answering as" it meant Mu's own model answering with that agent's scope and
// an empty prompt — near enough the default assistant, silently. The picker
// filtered them out and the rail opened them on the Connect page instead of a
// conversation.
//
// The kind is gone, so both special cases are gone with it. What is left worth
// holding is that nothing reintroduces a filter or a fork: an agent you made is
// an agent you can pick and an agent you can open.

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

// The chat picker offers every agent the account has.
func TestTheChatPickerOffersEveryAgent(t *testing.T) {
	src := readAgentSource(t, "../internal/app/chat.go")
	if strings.Contains(src, "a.kind!=='external'") {
		t.Error("the chat picker still filters on kind, which no longer distinguishes " +
			"anything — every agent runs here")
	}
}

// Clicking one on the rail opens the conversation, not a page about how to
// reach it.
func TestTheRailOpensAnAgentOnItsConversation(t *testing.T) {
	src := readAgentSource(t, "agents.go")
	if !strings.Contains(src, `onclick="muAgentOpen(`) {
		t.Fatal("the rail rows do not go through muAgentOpen")
	}
	// muAgentOpen used to branch to /agent/connect for an external agent. The
	// rail's own rows must not carry that jump any more.
	i := strings.Index(src, "function muAgentOpen(")
	if i < 0 {
		t.Fatal("muAgentOpen is gone; the rail has no opener")
	}
	end := strings.Index(src[i:], "\n}")
	if end < 0 {
		t.Fatal("muAgentOpen is never closed")
	}
	if strings.Contains(src[i:i+end], "/agent/connect") {
		t.Error("opening an agent from the rail still lands on the Connect page " +
			"for some of them")
	}
}
