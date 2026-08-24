package inbox

// One column, and everything said on the conversation in it.
//
// This file used to assert the opposite: two columns, and a split() that pulled
// the agent's messages out of the thread to fill the second one. The argument
// was that a mail thread reads badly with your own instructions interleaved
// through it — which was true of the control they were interleaved by, a box
// you typed in and waited at. That control is gone, so there is nothing left to
// hold apart: an agent's answer is a message on the conversation, like every
// other message on it.
//
// What must not come back is a second panel. Not because two columns are ugly,
// but because the panel was a chat, and a chat on this page argues against the
// page — the inbox is where things turn up whether or not you are in it.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

func TestTheConversationPageIsOneColumn(t *testing.T) {
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

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/inbox?id="+th.ID, nil)
	conversation(w, r, who, th.ID)
	page := w.Body.String()

	// Everything said on the conversation is on the page, whoever said it.
	// The agent's answer is not filed somewhere else — it is what arrived back.
	for _, want := range []string{
		"Are you free Tuesday?",
		"Book the meeting room",
		"Henrik is asking about Tuesday.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the conversation page is missing %q", want)
		}
	}

	// And there is no second panel to put any of it in.
	for _, gone := range []string{`ib-panels`, `ib-pane-agent`, `ib-chat`} {
		if strings.Contains(page, gone) {
			t.Errorf("the agent panel is back (%q) — it was a chat on the page "+
				"that exists so nobody has to wait at one", gone)
		}
	}

	// The way to hand it over is still there, because that is the half worth
	// having: it makes work and you close the tab. It is a button beside Reply
	// now rather than a permanent textarea under the thread — see actionBar.
	if !strings.Contains(page, "Assign to agent") {
		t.Error("there is no way to hand the conversation to an agent")
	}
}
