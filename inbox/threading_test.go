package inbox

// A reply has to be a reply on the other side too.
//
// Sending from /inbox called mail.SendOut with an empty replyTo — and SendOut
// had no references argument to pass anyway — so an answer left this instance
// with neither In-Reply-To nor References on it. It was filed correctly here
// and arrived in Gmail as a brand new conversation, sitting next to the thread
// it was answering. The record was right and everybody else's was wrong, which
// is the half nobody can see from inside.

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// landed files a message on a conversation as if it came in over SMTP.
func landed(t *testing.T, owner, threadID, from, ref, text string) {
	t.Helper()
	thread.Add(thread.Message{
		Thread: threadID, Account: owner, Text: text,
		From: from, Ref: ref,
	})
}

// The message being answered is named, and the whole chain with it.
func TestAReplyNamesTheMessageItAnswers(t *testing.T) {
	const who = "chain-owner"
	th := thread.Open(who, mailClient, "<a@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	landed(t, who, th.ID, "henrik@example.com", "<a@example.com>", "first")
	landed(t, who, th.ID, "henrik@example.com", "<b@example.com>", "second")

	inReplyTo, references := threadChain(who, th.ID)
	if inReplyTo != "<b@example.com>" {
		t.Errorf("In-Reply-To is %q — a reply answers the newest message, not the "+
			"one the conversation started with", inReplyTo)
	}
	// Oldest first, which is what RFC 5322 asks for and the order a client
	// rebuilds a thread from.
	if references != "<a@example.com> <b@example.com>" {
		t.Errorf("References is %q, want the chain oldest first", references)
	}
}

// A message with no Message-ID contributes nothing and does not become the
// parent — an agent's own answer is recorded without one, and threading a reply
// under it would name an id no client has ever seen.
func TestAMessageWithNoIDIsNotTheParent(t *testing.T) {
	const who = "chain-noref"
	th := thread.Open(who, mailClient, "<q@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	landed(t, who, th.ID, "henrik@example.com", "<q@example.com>", "the question")
	// The agent answering in the record, with nothing from a mail server.
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RoleAgent,
		Text: "the answer"})

	inReplyTo, references := threadChain(who, th.ID)
	if inReplyTo != "<q@example.com>" {
		t.Errorf("In-Reply-To is %q, want the last message that actually had an id", inReplyTo)
	}
	if strings.TrimSpace(references) != "<q@example.com>" {
		t.Errorf("References is %q — an empty ref made it into the header", references)
	}
}

// A conversation with no mail in it has no chain to join, and must not invent
// one: a chat or a WhatsApp exchange carries no Message-IDs.
func TestAConversationWithNoMailHasNoChain(t *testing.T) {
	const who = "chain-web"
	th := thread.Open(who, thread.WebClient, "web-1")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, Text: "typed here"})

	if in, refs := threadChain(who, th.ID); in != "" || refs != "" {
		t.Errorf("threadChain invented a chain: %q / %q", in, refs)
	}
}

// A new message is not a reply.
func TestANewMessageHasNoChain(t *testing.T) {
	if in, refs := threadChain("chain-none", ""); in != "" || refs != "" {
		t.Errorf("a message on no conversation got %q / %q", in, refs)
	}
}

// The subject says it is an answer, once.
func TestTheSubjectIsMarkedAsAReplyOnce(t *testing.T) {
	for _, tc := range []struct{ subject, in, want string }{
		{"Lunch", "<a@example.com>", "Re: Lunch"},
		{"Re: Lunch", "<a@example.com>", "Re: Lunch"},
		{"RE: Lunch", "<a@example.com>", "RE: Lunch"},
		{"Lunch", "", "Lunch"}, // not a reply
		{"", "<a@example.com>", ""},
	} {
		if got := replySubject(tc.subject, tc.in); got != tc.want {
			t.Errorf("replySubject(%q, %q) = %q, want %q", tc.subject, tc.in, got, tc.want)
		}
	}
}

// The chain is bounded. References grows by one id per message and a thread
// that has run for months would otherwise carry a header longer than the
// message.
func TestTheChainIsBounded(t *testing.T) {
	const who = "chain-long"
	th := thread.Open(who, mailClient, "<0@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	for i := 0; i < chainDepth+20; i++ {
		landed(t, who, th.ID, "henrik@example.com",
			"<"+strings.Repeat("x", i+1)+"@example.com>", "message")
	}
	_, references := threadChain(who, th.ID)
	if n := len(strings.Fields(references)); n > chainDepth {
		t.Errorf("%d ids in References, cap is %d", n, chainDepth)
	}
}
