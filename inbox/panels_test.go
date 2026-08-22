package inbox

// The two columns, and which messages belong in each.
//
// One conversation holds two exchanges — the mail, and what the owner told the
// agent to do about it — and the thing that must not break is the second one
// swallowing the first. A split that empties the reading column is a page with
// nothing on it, so that case is asserted rather than trusted.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

// A mail thread with an instruction on it: what arrived goes left, what was
// said to the agent goes right.
func TestTheAgentAsideIsSplitFromTheCorrespondence(t *testing.T) {
	msgs := []thread.Message{
		{Role: thread.RolePerson, Text: "Are you free Tuesday?", From: "henrik@example.com", Ref: "<1@example.com>"},
		{Role: thread.RolePerson, Text: "Yes, Tuesday works.", Ref: "<2@micro.mu>"},
		{Role: thread.RolePerson, Text: "Summarise this"},
		{Role: thread.RoleAgent, Text: "Henrik is asking about Tuesday."},
	}

	conv, aside := split(msgs)
	if len(conv) != 2 {
		t.Fatalf("the correspondence is %d messages, want the two that were sent: %+v", len(conv), conv)
	}
	if conv[0].Text != "Are you free Tuesday?" || conv[1].Text != "Yes, Tuesday works." {
		t.Errorf("the wrong messages are in the reading column: %+v", conv)
	}
	if len(aside) != 2 {
		t.Fatalf("the aside is %d messages, want the instruction and the answer: %+v", len(aside), aside)
	}
	if aside[0].Text != "Summarise this" || aside[1].Role != thread.RoleAgent {
		t.Errorf("the wrong messages are in the agent column: %+v", aside)
	}
}

// A conversation that is only you and the agent — a chat, or a thread nothing
// was ever sent on — is not split. Two columns with an empty one is a bug that
// looks like a design.
func TestAConversationWithNoCorrespondenceIsNotSplit(t *testing.T) {
	msgs := []thread.Message{
		{Role: thread.RolePerson, Text: "brief me"},
		{Role: thread.RoleAgent, Text: "Here is the briefing."},
	}
	conv, aside := split(msgs)
	if len(conv) != 2 || aside != nil {
		t.Errorf("a chat was split: conv=%+v aside=%+v", conv, aside)
	}
}

// And the page draws both, with the agent's own name on its column rather than
// the word for its role.
func TestTheConversationPageDrawsBothPanels(t *testing.T) {
	const who = "panels-reader"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	th := thread.Open(who, mailClient, "<panels@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Name(who, th.ID, "Tuesday")
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com", Ref: "<p1@example.com>"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Book the meeting room"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RoleAgent,
		Text: "Henrik is asking about Tuesday."})

	old := Act
	Act = func(accountID, threadID, ask string) error { return nil }
	defer func() { Act = old }()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/inbox?id="+th.ID, nil)
	conversation(w, r, who, th.ID)
	page := w.Body.String()

	for _, want := range []string{
		`class="ib-panels"`,
		`class="ib-pane ib-pane-conv"`,
		`class="ib-pane ib-pane-agent"`,
		"Are you free Tuesday?",
		"Henrik is asking about Tuesday.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the conversation page is missing %q", want)
		}
	}

	// The instruction is in the agent column and not in the mail thread, which
	// is the whole point of splitting them.
	left, right, ok := strings.Cut(page, `ib-pane ib-pane-agent`)
	if !ok {
		t.Fatal("no agent panel to cut at")
	}
	if strings.Contains(left, "Book the meeting room") {
		t.Error("the instruction was drawn in the mail thread")
	}
	if !strings.Contains(right, "Book the meeting room") {
		t.Error("the instruction is not in the agent panel")
	}
}
