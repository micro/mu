package mail

// What arrives by mail is a message, not a page of markup.
//
// Two things went wrong at once and both ended up in the system of record,
// where they are hard to see and hard to undo: the question handed to the agent
// was the sender's HTML, and every message ever sent to agent@ was filed as one
// conversation.

import (
	"os"
	"strings"
	"testing"

	"mu/service/mail"
)

// Each new message opens its own conversation; a reply lands on the one it
// answers.
//
// The key used to be the address written to, so everything anybody sent to
// agent@<domain> was a single thread: a stranger's question and yours filed
// together, and the agent handed both as context for answering either.
func TestANewMessageIsANewConversation(t *testing.T) {
	first := chainKey(mail.InboundMail{
		To: "agent@micro.mu", From: "asim@aslam.me", MessageID: "<one@phone>"})
	second := chainKey(mail.InboundMail{
		To: "agent@micro.mu", From: "asim@aslam.me", MessageID: "<two@phone>"})
	if first == second {
		t.Fatalf("two unrelated messages to the same address share a conversation (%q) — "+
			"everyone who writes in lands in one thread, with each other's history", first)
	}

	// A reply keys on the root of the chain, whichever turn it answers, so a
	// four-message exchange stays one conversation.
	for _, m := range []mail.InboundMail{
		{To: "agent@micro.mu", MessageID: "<three@phone>", InReplyTo: "<one@phone>"},
		{To: "agent@micro.mu", MessageID: "<four@phone>",
			References: "<one@phone> <three@phone>"},
	} {
		if got := chainKey(m); got != first {
			t.Errorf("a reply keyed %q, want the chain root %q", got, first)
		}
	}

	// Nothing to key on at all still keeps two strangers apart.
	a := chainKey(mail.InboundMail{To: "agent@micro.mu", From: "someone@example.com"})
	b := chainKey(mail.InboundMail{To: "agent@micro.mu", From: "other@example.com"})
	if a == b || a == "" {
		t.Error("a message with no ids at all put two strangers in one conversation")
	}
}

// The agent is asked what somebody wrote, not how their client sent it.
func TestTheAgentIsAskedTheMessageNotTheMarkup(t *testing.T) {
	src := readSource(t)

	if strings.Contains(src, "strings.TrimSpace(m.Body)\n") && !strings.Contains(src, "m.Text") {
		t.Fatal("the prompt is built from m.Body")
	}
	if !strings.Contains(src, "if b := strings.TrimSpace(m.Text); b != \"\"") {
		t.Error(`the prompt is not built from m.Text — a mail composed on a phone arrives as ` +
			`<div dir="auto">…</div>, and that went to the agent as the question and into the ` +
			`record as the conversation's name`)
	}
	// Body is still the fallback: messages recorded before Text existed have
	// nothing else, and prose that came in as plain text is identical in both.
	if !strings.Contains(src, "strings.TrimSpace(m.Body)") {
		t.Error("nothing falls back to Body, so a message stored before Text existed has no prompt")
	}
}

func readSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("mail.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
