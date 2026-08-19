package mail

// What an answer looks like on the wire.
//
// The agent's replies went out as a multipart/alternative whose HTML half was
// empty. That is a well-formed message, it passed every test here, and it
// arrives blank: a client renders the *last* alternative it understands, which
// was the empty one, so the text was always present and never shown. The first
// person to email the agent got an empty reply, in spam, from an address they
// had not written to.
//
// Building the message is a function of its own now so the wire format can be
// asserted on without relaying anything.

import (
	"strings"
	"testing"
)

func TestAnAnswerWithNoHTMLIsSentAsPlainText(t *testing.T) {
	msg, _ := buildExternal("Micro", "agent@micro.mu", "", "someone@example.com",
		"Re: rates", "£500 a day, and I am free Tuesday.", "", "<in-1@example.com>", "")
	got := string(msg)

	if strings.Contains(got, "multipart/alternative") {
		t.Error("an answer with no HTML still went as multipart/alternative — the " +
			"empty half is what the recipient's client displays, so the message " +
			"arrives blank")
	}
	if !strings.Contains(got, "Content-Type: text/plain") {
		t.Error("no text/plain part")
	}
	if !strings.Contains(got, "£500 a day, and I am free Tuesday.") {
		t.Fatal("the answer is not in the message at all")
	}
	// The body must come after the headers, or it is a header.
	head, body, ok := strings.Cut(got, "\r\n\r\n")
	if !ok {
		t.Fatal("no header/body separator")
	}
	if !strings.Contains(body, "£500") {
		t.Error("the answer did not land in the body")
	}
	if strings.Contains(head, "£500") {
		t.Error("the answer landed in the headers")
	}
}

func TestAnAnswerWithHTMLStillCarriesBoth(t *testing.T) {
	msg, _ := buildExternal("Micro", "agent@micro.mu", "", "someone@example.com",
		"Hello", "plain words", "<p>rich words</p>", "", "")
	got := string(msg)

	if !strings.Contains(got, "multipart/alternative") {
		t.Fatal("a message with both halves is no longer multipart")
	}
	for _, want := range []string{"text/plain", "text/html", "plain words", "rich words"} {
		if !strings.Contains(got, want) {
			t.Errorf("the multipart message is missing %q", want)
		}
	}
}

// A reply carries the whole chain, not just its parent.
//
// Repeating only the parent is enough for the second message in a thread and
// loses it by the fourth, which is where a client gives up and files the
// answers as unrelated mail.
func TestAReplyCarriesTheWholeReferenceChain(t *testing.T) {
	msg, _ := buildExternal("Micro", "agent@micro.mu", "", "someone@example.com",
		"Re: hello", "answer", "", "<third@example.com>",
		"<first@example.com> <second@example.com>")
	got := string(msg)

	if !strings.Contains(got, "In-Reply-To: <third@example.com>") {
		t.Error("the reply does not name the message it answers")
	}
	refs := headerOf(t, got, "References")
	for _, want := range []string{"<first@example.com>", "<second@example.com>", "<third@example.com>"} {
		if !strings.Contains(refs, want) {
			t.Errorf("References is %q, missing %s", refs, want)
		}
	}

	// With no chain supplied, the parent alone is still correct.
	msg2, _ := buildExternal("Micro", "agent@micro.mu", "", "someone@example.com",
		"Re: hi", "answer", "", "<only@example.com>", "")
	if refs := headerOf(t, string(msg2), "References"); !strings.Contains(refs, "<only@example.com>") {
		t.Errorf("References is %q with no chain supplied, want the parent", refs)
	}
}

// A message that answers nothing must not claim to.
func TestAFreshMessageHasNoThreadingHeaders(t *testing.T) {
	msg, _ := buildExternal("Mu", "no-reply@micro.mu", "", "someone@example.com",
		"Welcome", "hello", "", "", "")
	got := string(msg)
	for _, h := range []string{"In-Reply-To:", "References:"} {
		if strings.Contains(got, h) {
			t.Errorf("a message that answers nothing carries %s", h)
		}
	}
}

// headerOf returns one header's value from a built message.
func headerOf(t *testing.T, msg, name string) string {
	t.Helper()
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, name+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, name+":"))
		}
		if line == "" {
			break // end of headers
		}
	}
	t.Fatalf("no %s header in:\n%s", name, msg)
	return ""
}

// A reply-all is a well-formed message with everybody on it.
//
// Every agent mail reply goes through this path now, not only the copied-in
// ones — SendExternalReply delegates to it with no Cc. So a fault here is not a
// missing feature, it is every agent on the instance failing to answer its mail,
// and the seam exists because that has happened before: the answers went out as
// a multipart/alternative whose HTML half was empty, which is a well-formed
// message that displays as nothing.
func TestAReplyAllCarriesEverybody(t *testing.T) {
	msg, _ := buildExternalTo("Micro", "agent@micro.mu", "", "asim@example.com",
		[]string{"brother@example.net", "agent+markets@micro.mu"},
		"Re: the train", "when it leaves", "<p>when it leaves</p>", "<abc@example.com>", "")
	out := string(msg)

	head, body, ok := strings.Cut(out, "\r\n\r\n")
	if !ok {
		t.Fatal("no header/body separator, so this is not a message")
	}
	if !strings.Contains(head, "To: asim@example.com\r\n") {
		t.Error("the sender of the message being answered is not the To of the reply")
	}
	if !strings.Contains(head, "Cc: brother@example.net, agent+markets@micro.mu\r\n") {
		t.Errorf("the rest of the thread is not on the reply:\n%s", head)
	}
	// Threading, which is what keeps it in the conversation rather than
	// arriving as a separate message that says the same thing.
	if !strings.Contains(head, "In-Reply-To: <abc@example.com>") ||
		!strings.Contains(head, "References: <abc@example.com>") {
		t.Errorf("the reply is not threaded:\n%s", head)
	}
	if !strings.Contains(body, "when it leaves") {
		t.Error("the body did not survive")
	}
}

// And with nobody copied there is no empty Cc, which some servers reject and
// every client renders as a stray blank recipient.
func TestAnOrdinaryReplyHasNoCcHeader(t *testing.T) {
	msg, _ := buildExternalTo("Micro", "agent@micro.mu", "", "asim@example.com", nil,
		"Re: hello", "hi", "<p>hi</p>", "", "")
	if strings.Contains(string(msg), "Cc:") {
		t.Error("a one-to-one reply carries an empty Cc header")
	}
}
