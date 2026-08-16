package agent

// A thread that arrived by mail is one conversation.
//
// Before this, every message that woke an agent started a run with no parent:
// three emails were three strangers answering in sequence, and the second reply
// could not see the first. These pin the two halves of the fix — finding the
// turn a message continues, and handing the agent what was already said.

import (
	"testing"
	"time"
)

// mailTurn writes a finished turn as answerMail would, and returns its id.
func mailTurn(t *testing.T, account, parent, prompt, answer, inboundID, replyID string) string {
	t.Helper()
	id := Record(Recorded{
		Account: account, Source: FromMail, Trigger: "email from someone@example.com",
		Prompt: prompt, Answer: answer, Parent: parent,
		Via:     Via{Client: "mail", InboundID: inboundID, From: "someone@example.com"},
		Started: time.Now(),
	})
	if id == "" {
		t.Fatal("Record returned no id")
	}
	if replyID != "" {
		Delivered(id, replyID)
	}
	return id
}

func TestAReplyContinuesTheTurnItAnswers(t *testing.T) {
	const acc = "thread-reply"
	first := mailTurn(t, acc, "", "what are your rates?", "£500 a day.",
		"<in-1@example.com>", "<out-1@micro.mu>")

	// The follow-up answers what we sent, which is the ordinary case: a mail
	// client replies to the reply.
	if got := ContinuesMail(acc, "<out-1@micro.mu>", ""); got != first {
		t.Errorf("a reply to our answer started a new conversation (got %q, want %q)", got, first)
	}
	// Some clients send only References, and some name the original message
	// rather than the reply. Both still have to find it.
	if got := ContinuesMail(acc, "", "<other@x.com> <out-1@micro.mu>"); got != first {
		t.Errorf("References alone did not find the turn (got %q)", got)
	}
	if got := ContinuesMail(acc, "<in-1@example.com>", ""); got != first {
		t.Errorf("a reply naming the original inbound message did not find the turn (got %q)", got)
	}
}

func TestAMessageThatAnswersNothingStartsAConversation(t *testing.T) {
	const acc = "thread-fresh"
	mailTurn(t, acc, "", "hello", "hi", "<in-a@example.com>", "<out-a@micro.mu>")

	if got := ContinuesMail(acc, "", ""); got != "" {
		t.Errorf("a message with no threading headers was attached to %q", got)
	}
	if got := ContinuesMail(acc, "<never-seen@example.com>", ""); got != "" {
		t.Errorf("a message answering something we never sent was attached to %q", got)
	}
}

// A long thread continues from its head, not its middle.
//
// Mail clients quote the whole chain in References, so every turn's id is in
// there. Matching the first one found would hang each new message off the
// oldest turn and the conversation would fan out into a bush.
func TestALongThreadContinuesFromItsNewestTurn(t *testing.T) {
	const acc = "thread-head"
	first := mailTurn(t, acc, "", "one", "1", "<in-1@x.com>", "<out-1@micro.mu>")
	// Explicit timestamps: the two turns are written in the same millisecond
	// otherwise, and "newest" would be whichever the map iterated to.
	updateFlow(first, func(f *Flow) { f.CreatedAt = time.Now().Add(-time.Hour) })
	second := mailTurn(t, acc, first, "two", "2", "<in-2@x.com>", "<out-2@micro.mu>")

	refs := "<out-1@micro.mu> <out-2@micro.mu>"
	if got := ContinuesMail(acc, "<out-2@micro.mu>", refs); got != second {
		t.Errorf("continued from %q, want the newest turn %q — a client quotes the "+
			"whole chain, so matching the oldest would fan the thread into a bush",
			got, second)
	}
}

// A thread belongs to one account and cannot be continued from another.
//
// The lookup is by message id, and a message id is a string somebody else's
// mail server wrote. If it were not scoped to the account, quoting an id would
// attach a stranger's message to somebody else's conversation — and the history
// of that conversation would then be handed to the model answering them.
func TestAThreadCannotBeContinuedFromAnotherAccount(t *testing.T) {
	const mine, yours = "thread-mine", "thread-yours"
	mailTurn(t, mine, "", "private", "confidential", "<in-p@x.com>", "<out-p@micro.mu>")

	if got := ContinuesMail(yours, "<out-p@micro.mu>", ""); got != "" {
		t.Fatalf("another account continued my thread (got %q) — quoting a message id "+
			"would hand them its history", got)
	}
}

// The agent is told what was already said.
//
// This is what separates a thread from a chain that merely looks like one on a
// page: without history the agent meets every message as a stranger.
func TestTheAgentIsGivenWhatWasAlreadySaid(t *testing.T) {
	const acc = "thread-history"
	first := mailTurn(t, acc, "", "book me a table", "which night?", "<h1@x.com>", "<r1@micro.mu>")
	updateFlow(first, func(f *Flow) { f.CreatedAt = time.Now().Add(-time.Hour) })
	second := mailTurn(t, acc, first, "friday", "done, 8pm", "<h2@x.com>", "<r2@micro.mu>")

	h := ThreadHistory(acc, second, 10)
	if len(h) != 4 {
		t.Fatalf("history has %d messages, want 4 (two turns, both sides): %+v", len(h), h)
	}
	// Oldest first, alternating, which is the order a model expects.
	want := []QueryMessage{
		{Role: "user", Text: "book me a table"},
		{Role: "assistant", Text: "which night?"},
		{Role: "user", Text: "friday"},
		{Role: "assistant", Text: "done, 8pm"},
	}
	for i, w := range want {
		if h[i] != w {
			t.Errorf("message %d is %+v, want %+v", i, h[i], w)
		}
	}

	// Bounded, so a thread somebody has added to for a month does not cost more
	// in prompt than the answer is worth. The recent turns are the ones kept.
	if trimmed := ThreadHistory(acc, second, 2); len(trimmed) != 2 ||
		trimmed[1].Text != "done, 8pm" {
		t.Errorf("trimmed history is %+v, want the last two messages", trimmed)
	}
	if got := ThreadHistory(acc, "", 10); got != nil {
		t.Errorf("a conversation with no parent has history %+v, want none", got)
	}
}

func TestTheReplyIdIsRecordedSoTheNextReplyFindsIt(t *testing.T) {
	const acc = "thread-delivered"
	id := mailTurn(t, acc, "", "q", "a", "<in-d@x.com>", "")

	if got := ContinuesMail(acc, "<out-d@micro.mu>", ""); got != "" {
		t.Fatal("found a turn by a reply id that was never recorded")
	}
	Delivered(id, "<out-d@micro.mu>")
	if got := ContinuesMail(acc, "<out-d@micro.mu>", ""); got != id {
		t.Errorf("after Delivered, the reply does not find its turn (got %q, want %q)", got, id)
	}

	// Nothing to record is not an error, and must not blank what is there.
	Delivered(id, "   ")
	if f := getFlow(id); f == nil || f.Via.ReplyID != "<out-d@micro.mu>" {
		t.Error("an empty Delivered wiped the recorded reply id")
	}
}
