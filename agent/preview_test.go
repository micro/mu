package agent

// What Home says you have working.

import (
	"strings"
	"testing"
)

// The agent every account already has is on the list.
//
// Preview read Agents(owner), which is an account's *own* agents, so the one
// that answers agent@ and that the chat talks to was missing from the page
// whose job is to say what you have working. /agents had the same bug and its
// fix is recorded there: leaving Micro off "meant a new account opened /agents
// and was told it had none, which is false".
func TestThePreviewIncludesTheDefaultAgent(t *testing.T) {
	got := Preview("preview-default")
	if got == "" {
		t.Fatal("an account with the default agent got an empty list")
	}

	want := Platform(DefaultPlatformAgent)
	if want == nil {
		t.Skip("no platform registry in this build")
	}
	if !strings.Contains(got, want.Name) {
		t.Errorf("the default agent is not on Home:\n%s", got)
	}
	// And it goes where /agents sends it, or the two pages name one agent and
	// point at two.
	if !strings.Contains(got, `href="/agent/`+DefaultPlatformAgent+`"`) {
		t.Errorf("the default agent does not link to its own page:\n%s", got)
	}
}
