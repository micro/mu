package agent

import (
	"testing"

	"mu/internal/thread"
)

// A client with no key of its own still gets a conversation.
//
// thread.Open returns nil for an empty key, and a nil thread makes every
// recording call a no-op. POST /agent/<name> has no key until it has been told
// one, so a first call wrote nothing to the record and returned no thread id —
// the field its own documentation calls "returned always, because a caller that
// wants a second turn needs it and has no other way to learn it".
//
// Verified live before the fix: the response carried text, agent and flow, and
// no thread at all.
func TestAKeylessClientStillGetsAConversation(t *testing.T) {
	const who = "ask-keyless"
	// No provider here, so the model call fails — and that is fine: the
	// conversation is opened and the question recorded before the model is
	// asked, and Ask returns the thread id even on error. What is under test is
	// the record, not the answer.
	res, _ := Ask(AskRequest{
		Account: who,
		Client:  thread.WebClient,
		Text:    "what is the bitcoin price",
		Trigger: "api",
	})
	if res.Thread == "" {
		t.Fatal("no thread id came back, so a program can never continue the " +
			"conversation it just started")
	}

	// And the turn is in the record, which is the other half: an API
	// conversation appeared in neither /inbox nor /recall.
	if got := thread.Get(who, res.Thread); got == nil {
		t.Fatal("the conversation was not written to the record")
	}
	msgs := thread.Messages(who, res.Thread, 10)
	if len(msgs) == 0 {
		t.Fatal("the question was not written down")
	}
	if msgs[0].Text != "what is the bitcoin price" {
		t.Errorf("the record holds %q, want the question that was asked", msgs[0].Text)
	}

	// A second call naming it continues the same conversation rather than
	// opening another.
	again, _ := Ask(AskRequest{
		Account: who,
		Client:  thread.WebClient,
		On:      res.Thread,
		Text:    "and ethereum?",
		Trigger: "api",
	})
	if again.Thread != res.Thread {
		t.Errorf("the second turn landed on %q, want %q", again.Thread, res.Thread)
	}
}

// Two fresh calls are two conversations, not one shared by every caller with
// no key — which is what a fixed placeholder key would have produced.
func TestTwoKeylessCallsAreTwoConversations(t *testing.T) {
	const who = "ask-keyless-two"
	one, _ := Ask(AskRequest{Account: who, Client: thread.WebClient, Text: "first"})
	two, _ := Ask(AskRequest{Account: who, Client: thread.WebClient, Text: "second"})
	if one.Thread == "" || two.Thread == "" {
		t.Fatal("a call came back with no conversation")
	}
	if one.Thread == two.Thread {
		t.Error("two unrelated questions were filed on one conversation")
	}
}
