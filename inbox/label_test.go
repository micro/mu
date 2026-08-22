package inbox

// A mailbox is named after an agent, and an id is not a name.

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// An agent that has been deleted leaves conversations behind. They belong in
// All; they do not deserve a mailbox labelled with the id of a thing nobody can
// open.
//
// The id used to be the fallback, and what it produced was a rail offering
// "47b6428c-fa8a-4610-a302-45dbc992ad5d" as somewhere to click — three of four
// mailboxes named after rows in a file.
func TestAVanishedAgentGetsNoMailbox(t *testing.T) {
	const who = "inbox-vanished"
	withRoster(t, who, Agent{ID: "here", Name: "Research", Tag: "research"})

	said(t, who, "mail", "<a@example.com>", "here", "found three papers")
	said(t, who, "mail", "<b@example.com>", "47b6428c-fa8a-4610-a302-45dbc992ad5d", "an older conversation")

	for _, box := range Mailboxes(who) {
		if strings.Contains(box.Label, "47b6428c") {
			t.Errorf("the rail offers a mailbox called %q", box.Label)
		}
	}
	if n := len(Mailboxes(who)); n != 2 { // All, and Research
		t.Errorf("%d mailboxes, want All and Research", n)
	}

	// And the conversation is not lost — All still has it.
	body := listBody(t, "/inbox", who, "")
	if !strings.Contains(body, "an older conversation") {
		t.Error("the conversation went missing with its agent")
	}
	if strings.Contains(body, "47b6428c") {
		t.Error("the switcher is showing an id")
	}
}

// With no agent package wired in at all there are no mailboxes, rather than one
// per raw id.
func TestWithoutTheRosterThereAreNoMailboxes(t *testing.T) {
	const who = "inbox-no-roster"
	Agents, AgentName = nil, nil

	said(t, who, thread.WebClient, "chat", "some-agent-id", "hello")

	if got := Mailboxes(who); len(got) != 0 {
		t.Errorf("%d mailboxes with nothing to name them: %+v", len(got), got)
	}
}
