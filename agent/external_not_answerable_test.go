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
