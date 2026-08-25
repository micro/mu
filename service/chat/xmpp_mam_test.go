package chat

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// A client that opens a conversation sees what was said before.
//
// This is the whole feature. Everything up to now let a client send and receive
// and showed it nothing from yesterday, which is a walkie-talkie rather than a
// chat client — while the record held every word, because every client here
// writes to it on every turn. The messages were never missing; there was no way
// to ask.
func TestOpeningAConversationShowsWhatWasSaid(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "mamreader")

	// Yesterday, on any client — this is the record, not a chat fixture.
	th := thread.Open("mamreader", thread.ChatClient,
		xmppRoom("mamreader@example.test", "agent@example.test"))
	if th == nil {
		t.Fatal("could not open the conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: "mamreader",
		Role: thread.RolePerson, Text: "what is the bitcoin price"})
	thread.Add(thread.Message{Thread: th.ID, Account: "mamreader",
		Role: thread.RoleAgent, Text: "about sixty thousand dollars"})

	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)

	c.write(mamQuery("q1", "agent@example.test", ""))
	got := c.until(t, "</iq>")

	for _, want := range []string{"what is the bitcoin price", "about sixty thousand dollars"} {
		if !strings.Contains(got, want) {
			t.Errorf("the archive did not return %q — a client opening this "+
				"conversation shows an empty screen:\n%s", want, clip(got))
		}
	}
	// Wrapped the way the XEP says, or a client will not recognise it as history
	// and will render it as two messages arriving now.
	if !strings.Contains(got, "urn:xmpp:forward:0") || !strings.Contains(got, "urn:xmpp:delay") {
		t.Errorf("results are not forwarded with a timestamp: %s", clip(got))
	}
	// And the agent's answer has to come from the agent, not from you.
	if !strings.Contains(got, "from='agent@example.test'") {
		t.Errorf("the agent's reply is not addressed from the agent, so a client "+
			"draws it as your own message: %s", clip(got))
	}
	if !strings.Contains(got, "<fin ") {
		t.Errorf("the query never finished, so a client waits forever: %s", clip(got))
	}
}

// A query for one conversation returns only that conversation.
//
// The filter arrives inside a data form rather than as an attribute, which is
// the kind of thing that gets pattern-matched loosely — and a loose match here
// hands somebody every conversation they have ever had when they asked for one.
func TestAnArchiveQueryDoesNotLeakAnotherConversation(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "mamscoped")

	withAgent := thread.Open("mamscoped", thread.ChatClient,
		xmppRoom("mamscoped@example.test", "agent@example.test"))
	thread.Add(thread.Message{Thread: withAgent.ID, Account: "mamscoped",
		Role: thread.RolePerson, Text: "asked the agent something"})

	withPerson := thread.Open("mamscoped", thread.ChatClient,
		xmppRoom("mamscoped@example.test", "someone@example.test"))
	thread.Add(thread.Message{Thread: withPerson.ID, Account: "mamscoped",
		Role: thread.RolePerson, Text: "a private word with someone else"})

	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)

	c.write(mamQuery("q1", "agent@example.test", ""))
	got := c.until(t, "</iq>")

	if !strings.Contains(got, "asked the agent something") {
		t.Fatalf("the conversation asked for is missing: %s", clip(got))
	}
	if strings.Contains(got, "a private word with someone else") {
		t.Error("a query for one conversation returned another one")
	}
}

// A page is bounded, and says whether there is more behind it.
//
// Saying complete when there is more is how a client stops asking and a
// conversation appears to begin in the middle.
func TestAPageSaysWhetherThereIsMore(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "mampager")

	th := thread.Open("mampager", thread.ChatClient,
		xmppRoom("mampager@example.test", "agent@example.test"))
	for _, s := range []string{"one", "two", "three", "four", "five"} {
		thread.Add(thread.Message{Thread: th.ID, Account: "mampager",
			Role: thread.RolePerson, Text: "message " + s})
	}

	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)

	c.write(mamQuery("q1", "agent@example.test", "2"))
	got := c.until(t, "</iq>")

	if n := strings.Count(got, "<forwarded"); n != 2 {
		t.Errorf("asked for 2 and got %d — a page size nobody honours is a "+
			"stanza storm on a real account", n)
	}
	if !strings.Contains(got, `complete='false'`) {
		t.Error("the page claims to be the whole archive with three messages " +
			"behind it, so the client will never scroll up")
	}
	// The newest, because that is what opening a conversation asks for.
	if !strings.Contains(got, "message five") {
		t.Errorf("the page is not the most recent one: %s", clip(got))
	}
}

// A client is told the archive exists.
//
// Nothing above works without this: Conversations will not ask a server that
// has not said it has an archive, so an archive with no disco entry is an
// archive nobody reads.
func TestAClientIsToldTheArchiveExists(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "mamdisco")

	c := dial(t)
	defer c.Close()
	c.handshake(t, acc.ID, token)

	c.write(`<iq type='get' id='d1' to='example.test'>` +
		`<query xmlns='http://jabber.org/protocol/disco#info'/></iq>`)
	got := c.until(t, "</iq>")

	if !strings.Contains(got, "urn:xmpp:mam:2") {
		t.Errorf("the archive is not advertised, so no client will ask for "+
			"it: %s", clip(got))
	}
	if !strings.Contains(got, "<identity") {
		t.Errorf("no identity, which is what a client uses to know what it is "+
			"talking to: %s", clip(got))
	}
}

// mamQuery is what a client sends to open a conversation.
func mamQuery(id, with, max string) string {
	set := ""
	if max != "" {
		set = `<set xmlns='http://jabber.org/protocol/rsm'><max>` + max + `</max><before/></set>`
	}
	return `<iq type='set' id='` + id + `'><query xmlns='urn:xmpp:mam:2' queryid='` + id + `'>` +
		`<x xmlns='jabber:x:data' type='submit'>` +
		`<field var='FORM_TYPE' type='hidden'><value>urn:xmpp:mam:2</value></field>` +
		`<field var='with'><value>` + with + `</value></field>` +
		`</x>` + set + `</query></iq>`
}
