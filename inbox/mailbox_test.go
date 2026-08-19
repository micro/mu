package inbox

// What makes this a mailbox rather than a list.

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/thread"
)

// arrived is a conversation somebody else started: a message with a From,
// which is what makes it something waiting for you rather than something you
// wrote. said() has no From, so it is the owner speaking and is read on
// arrival — see thread.Add.
func arrived(t *testing.T, owner, client, key, agentID, from, text string) *thread.Thread {
	t.Helper()
	th := thread.Open(owner, client, key)
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	if agentID != "" {
		thread.SetAgent(owner, th.ID, agentID)
	}
	thread.Add(thread.Message{Thread: th.ID, Account: owner, Text: text, From: from})
	return th
}

// The inbox is what arrived; a chat you started here is not in it.
//
// These were two lists of the same conversations — /inbox and the rail on
// /agent both read the whole record — so neither page could be described in a
// sentence, which is exactly how it read.
func TestTheInboxIsWhatArrived(t *testing.T) {
	const who = "mailbox-arrived"
	said(t, who, "mail", "<a@example.com>", "", "about the invoice")
	said(t, who, thread.WebClient, "chat", "", "what are the markets doing")
	said(t, who, thread.CLIClient, "term", "", "run the briefing")

	body := listBody(t, "/inbox", who, "")
	if !strings.Contains(body, "about the invoice") {
		t.Error("mail is not in the inbox")
	}
	for _, started := range []string{"what are the markets doing", "run the briefing"} {
		if strings.Contains(body, started) {
			t.Errorf("a conversation started here is in the inbox: %q", started)
		}
	}
}

// A row you have not read looks different from one you have. Without that the
// page is a log — read top to bottom every time, because nothing says which of
// these you have dealt with.
func TestUnreadRowsAreMarked(t *testing.T) {
	const who = "mailbox-unread"
	th := arrived(t, who, "mail", "<b@example.com>", "", "them@example.com", "the quarterly numbers")

	// The markup, not the stylesheet — mu.css always carries the rule.
	const marked = `class="ib-row unseen"`
	if body := listBody(t, "/inbox", who, ""); !strings.Contains(body, marked) {
		t.Errorf("a conversation nobody has opened is not marked unread:\n%s", body)
	}

	thread.MarkSeen(who, th.ID)
	if body := listBody(t, "/inbox", who, ""); strings.Contains(body, marked) {
		t.Error("a conversation that has been read is still marked unread")
	}
}

// Your own words are read the moment you write them. An agent's answer, and
// anything somebody else sends, is not.
func TestWhatCountsAsUnread(t *testing.T) {
	const who = "mailbox-rule"

	mine := thread.Open(who, "mail", "<mine@example.com>")
	thread.Add(thread.Message{Thread: mine.ID, Account: who, Text: "I said this"})
	if got := thread.Get(who, mine.ID); thread.Unread(*got) {
		t.Error("something you wrote yourself is unread")
	}

	// The agent answers.
	thread.Add(thread.Message{Thread: mine.ID, Account: who,
		Role: thread.RoleAgent, Text: "and it answered"})
	if got := thread.Get(who, mine.ID); !thread.Unread(*got) {
		t.Error("an answer you have not seen is not unread")
	}

	// Somebody else writes in.
	theirs := thread.Open(who, "mail", "<theirs@example.com>")
	thread.Add(thread.Message{Thread: theirs.ID, Account: who,
		Text: "a stranger wrote", From: "someone@example.com"})
	if got := thread.Get(who, theirs.ID); !thread.Unread(*got) {
		t.Error("mail from somebody else is not unread")
	}
}

// Opening a conversation reads it, and there is a way to put it back — which is
// the half a mailbox is unusable without, because a thing you meant to come back
// to otherwise disappears into the pile.
func TestOpeningReadsItAndItCanBePutBack(t *testing.T) {
	const who = "mailbox-open"
	th := arrived(t, who, "mail", "<c@example.com>", "", "them@example.com", "please confirm")

	w := httptest.NewRecorder()
	conversation(w, httptest.NewRequest("GET", "/inbox?id="+url.QueryEscape(th.ID), nil), who, th.ID)
	body := w.Body.String()

	if got := thread.Get(who, th.ID); thread.Unread(*got) {
		t.Error("opening a conversation did not read it")
	}
	// And the control is offered, because it was unread when it was opened.
	if !strings.Contains(body, "Mark unread") {
		t.Errorf("no way to put it back:\n%s", body)
	}

	thread.MarkUnread(who, th.ID)
	if got := thread.Get(who, th.ID); !thread.Unread(*got) {
		t.Error("marking unread did nothing")
	}
}

// The control is not offered on something that was already read. Marking unread
// what you had already dealt with is a thing nobody does, and a control shown
// always is furniture on every page.
func TestNothingToPutBackOnSomethingAlreadyRead(t *testing.T) {
	const who = "mailbox-read"
	th := said(t, who, "mail", "<d@example.com>", "", "no action needed")
	thread.MarkSeen(who, th.ID)

	w := httptest.NewRecorder()
	conversation(w, httptest.NewRequest("GET", "/inbox?id="+url.QueryEscape(th.ID), nil), who, th.ID)
	if strings.Contains(w.Body.String(), "Mark unread") {
		t.Error("a conversation that was already read offers to be put back")
	}
}

// The rail carries the count, per mailbox and for the whole inbox — a number
// rather than a dot, because three and forty are different situations.
func TestTheRailCountsWhatIsWaiting(t *testing.T) {
	const who = "mailbox-count"
	AgentName = func(owner, id string) string {
		if id == "a1" {
			return "Research"
		}
		return ""
	}
	t.Cleanup(func() { AgentName = nil })

	arrived(t, who, "mail", "<e@example.com>", "a1", "them@example.com", "found three papers")
	read := arrived(t, who, "mail", "<f@example.com>", "a1", "them@example.com", "and one more")
	thread.MarkSeen(who, read.ID)
	arrived(t, who, "whatsapp", "44700900000", "", "44700900000", "are you around")

	if got := Unread(who); got != 2 {
		t.Errorf("%d unread, want the two nobody has opened", got)
	}

	boxes := Mailboxes(who)
	if len(boxes) == 0 {
		t.Fatal("no mailboxes")
	}
	if boxes[0].Label != "All" || boxes[0].Badge != "2" {
		t.Errorf("the whole inbox reads %q/%q, want All/2", boxes[0].Label, boxes[0].Badge)
	}
	for _, b := range boxes[1:] {
		if b.Label == "Research" && b.Badge != "1" {
			t.Errorf("the research box reads %q, want 1 — one of its two is read", b.Badge)
		}
	}
}

// A mailbox you cannot delete from is a list that only grows.
func TestAConversationCanBeDeleted(t *testing.T) {
	const who = "mailbox-delete"
	th := arrived(t, who, "mail", "<g@example.com>", "", "them@example.com", "delete me")

	w := httptest.NewRecorder()
	conversation(w, httptest.NewRequest("GET", "/inbox?id="+url.QueryEscape(th.ID), nil), who, th.ID)
	if !strings.Contains(w.Body.String(), "/inbox/delete") {
		t.Error("no way to delete a conversation")
	}

	form := url.Values{"id": {th.ID}}
	r := httptest.NewRequest("POST", "/inbox/delete", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	DeleteHandler(httptest.NewRecorder(), r)

	// No session on that request, so nothing went — the handler requires one.
	if thread.Get(who, th.ID) == nil {
		t.Error("a request with no session deleted a conversation")
	}
	// The store's own scoping is the guarantee that matters.
	thread.Delete("somebody-else", th.ID)
	if thread.Get(who, th.ID) == nil {
		t.Error("somebody else deleted this account's conversation")
	}
	thread.Delete(who, th.ID)
	if thread.Get(who, th.ID) != nil {
		t.Error("the owner could not delete their own conversation")
	}
}

// The briefing arrived as "Untitled": a conversation took its name from a
// person's message only, and the one thing the agent starts on its own is the
// briefing.
func TestAConversationTheAgentStartsHasAName(t *testing.T) {
	const who = "mailbox-subject"
	th := thread.Open(who, "digest", "digest-2026-08-19")
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RoleAgent,
		Text: "Daily Digest — 19 Aug 2026\n\nMarkets were quiet and the news was not."})

	got := thread.Get(who, th.ID)
	if got == nil {
		t.Fatal("no conversation")
	}
	if got.Subject != "Daily Digest — 19 Aug 2026" {
		t.Errorf("the briefing is called %q", got.Subject)
	}
}

// And a mail conversation is named by its subject line, not its subject line
// plus the opening of the body.
func TestAMailConversationIsNamedByItsSubject(t *testing.T) {
	const who = "mailbox-mailsubject"
	th := thread.Open(who, "mail", "<h@example.com>")
	thread.Add(thread.Message{Thread: th.ID, Account: who, From: "them@example.com",
		Text: "Invoice 4021\n\nAttached is this month's invoice, due on the 30th."})

	if got := thread.Get(who, th.ID); got.Subject != "Invoice 4021" {
		t.Errorf("the conversation is called %q", got.Subject)
	}
}

// A conversation says who it was from and who it was sent to, in full.
//
// Neither was visible: the sender was truncated into a 130px column and the
// recipient was never recorded at all. Which of your addresses a message
// arrived at is the fact that explains why it is in this inbox — you@ is you,
// you+research@ is one of your agents, agent@ is this instance's.
func TestAConversationSaysWhoItWasSentTo(t *testing.T) {
	const who = "mailbox-addresses"
	th := thread.Open(who, "mail", "<addr@example.com>")
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		From: "henrik@getdirectree.example.com",
		To:   "asim+research@micro.mu",
		Text: "About the thing"})

	w := httptest.NewRecorder()
	conversation(w, httptest.NewRequest("GET", "/inbox?id="+url.QueryEscape(th.ID), nil), who, th.ID)
	body := w.Body.String()

	for _, want := range []string{"henrik@getdirectree.example.com", "asim+research@micro.mu"} {
		if !strings.Contains(body, want) {
			t.Errorf("the conversation does not show %q in full:\n%s", want, body)
		}
	}
}

// Your own words carry no address line. "From: you, To: nobody" is furniture.
func TestYourOwnMessagesHaveNoAddressLine(t *testing.T) {
	if got := addressLine(thread.Message{Text: "mine"}); got != "" {
		t.Errorf("a message you wrote shows an address line: %q", got)
	}
}

// A long address is readable in the list, and the whole of it is one hover
// away. "henrik@getdirectree.co…" tells you nothing "henrik" does not, and the
// part cut off is the part that would have.
func TestALongSenderIsReadableInTheList(t *testing.T) {
	const who = "mailbox-longsender"
	th := arrived(t, who, "mail", "<long@example.com>", "",
		"henrik@getdirectree.example.com", "hello")

	body := listBody(t, "/inbox", who, "")
	if !strings.Contains(body, `title="henrik@getdirectree.example.com"`) {
		t.Errorf("the full address is not available on the row:\n%s", body)
	}
	if !strings.Contains(body, ">henrik<") {
		t.Errorf("the row does not show a readable name:\n%s", body)
	}
	_ = th
}
