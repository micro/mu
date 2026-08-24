package inbox

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/thread"
)

// The Assign button and the dialog it opens are never apart.
//
// They were, for one commit: ConversationView drew the button and only the
// inbox page drew the dialog, so opening a mail thread from /agent got a button
// that did nothing. That is the same shape as the agent page calling a function
// defined in a panel it had stopped rendering — a control whose other half is
// somebody else's job to remember.
//
// So conversationPane takes the dialog and draws the button only when it has
// one, and this holds both directions of that.
func TestTheAssignButtonAlwaysHasItsDialog(t *testing.T) {
	const who = "render-pair"
	th := thread.Open(who, mailClient, "<pair@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{
		Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Can you pull the quarterly numbers?", From: "henrik@example.com",
	})
	live := thread.Get(who, th.ID)
	msgs := thread.Messages(who, th.ID, MessagesShown)

	// Offered: both halves.
	r := httptest.NewRequest("GET", "/inbox?id="+th.ID, nil)
	with := conversationPane(who, live, msgs, false, false,
		assignDialog(r, th.ID, replyTo(who, live, msgs)))
	if !strings.Contains(with, "Assign to agent") {
		t.Error("no way to assign on the page that offers one")
	}
	if !strings.Contains(with, `id="ib-assign"`) {
		t.Error("the button is there and the dialog it opens is not")
	}
	// The opener has to be defined too, or the button throws.
	if !strings.Contains(with, "function muAssignOpen") {
		t.Error("muAssignOpen is called by the button and defined nowhere")
	}

	// Not offered: neither half, rather than a button that opens nothing.
	without := conversationPane(who, live, msgs, false, true, "")
	if strings.Contains(without, "Assign to agent") {
		t.Error("a page with no dialog is still drawing the button that opens it")
	}

	// And the conversation is not carrying a permanent textarea any more —
	// the whole point of moving it into a dialog.
	if before, _, _ := strings.Cut(with, "<dialog"); strings.Contains(before, "<textarea") {
		t.Error("there is still a textarea under the conversation")
	}
}
