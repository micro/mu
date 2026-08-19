package inbox

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/thread"
)

// said starts a conversation with an agent and puts a line in it.
func said(t *testing.T, owner, client, key, agentID, text string) *thread.Thread {
	t.Helper()
	th := thread.Open(owner, client, key)
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	if agentID != "" {
		thread.SetAgent(owner, th.ID, agentID)
	}
	thread.Add(thread.Message{Thread: th.ID, Account: owner, Text: text})
	return th
}

// The inbox lists conversations, whichever client each arrived on.
//
// That is what makes it one inbox rather than five: an email chain, a WhatsApp
// exchange and a chat on this page are the same kind of thing in the record.
func TestTheInboxListsEveryConversation(t *testing.T) {
	const who = "inbox-lister"
	said(t, who, "mail", "<a@example.com>", "", "about the invoice")
	said(t, who, "whatsapp", "44700900000", "", "are you around")

	body := listBody(t, "/inbox", who, "")
	for _, want := range []string{"about the invoice", "are you around"} {
		if !strings.Contains(body, want) {
			t.Errorf("the inbox is missing %q", want)
		}
	}
	// Where it happened, when that is not here.
	if !strings.Contains(body, "Email") || !strings.Contains(body, "WhatsApp") {
		t.Errorf("the rows do not say where they happened:\n%s", body)
	}
}

// An agent is a mailbox. What arrives for the research agent is its mail, not a
// slice of yours, so it gets a box of its own with a way in and out.
func TestEachAgentIsItsOwnMailbox(t *testing.T) {
	const who = "inbox-boxes"
	AgentName = func(owner, id string) string {
		switch id {
		case "a1":
			return "Research"
		case "a2":
			return "Briefer"
		}
		return ""
	}
	t.Cleanup(func() { AgentName = nil })

	said(t, who, "mail", "<r@example.com>", "a1", "found three papers")
	said(t, who, "mail", "<b@example.com>", "a2", "your morning brief")
	said(t, who, thread.WebClient, "plain", "", "just chatting")

	all := listBody(t, "/inbox", who, "")
	for _, want := range []string{`href="/inbox/research"`, `href="/inbox/briefer"`, `href="/inbox"`} {
		if !strings.Contains(all, want) {
			t.Errorf("no way to reach %s", want)
		}
	}

	one := listBody(t, "/inbox/research", who, "research")
	if !strings.Contains(one, "found three papers") {
		t.Error("the research box does not hold its own conversation")
	}
	for _, other := range []string{"your morning brief", "just chatting"} {
		if strings.Contains(one, other) {
			t.Errorf("the research box also shows %q", other)
		}
	}
}

// A switcher with one destination is a control that cannot do anything.
func TestNoSwitcherWhenNothingHasAnAgent(t *testing.T) {
	const who = "inbox-one-box"
	said(t, who, thread.WebClient, "only", "", "hello")

	// The markup, not the stylesheet — mu.css always carries the rule.
	if body := listBody(t, "/inbox", who, ""); strings.Contains(body, `<div class="ib-boxes">`) {
		t.Error("an account with no agent conversations is offered a switcher")
	}
}

// An empty box says which box is empty. The narrower fact is the true one, and
// the address is already on the page above it.
func TestAnEmptyBoxSaysWhichBoxIsEmpty(t *testing.T) {
	const who = "inbox-empty-box"
	// A distinctive phrase: the page shell has prose in it, and a common
	// word will match a comment rather than a row.
	said(t, who, "mail", "<x@example.com>", "", "zarquon the invoice")

	body := listBody(t, "/inbox/briefer", who, "briefer")
	if !strings.Contains(body, "briefer") {
		t.Errorf("an empty box does not name itself:\n%s", body)
	}
	if strings.Contains(body, "zarquon") {
		t.Error("an empty box is showing another box's conversation")
	}
}

func listBody(t *testing.T, path, accountID, box string) string {
	t.Helper()
	w := httptest.NewRecorder()
	list(w, httptest.NewRequest("GET", path, nil), accountID, box)
	return w.Body.String()
}
