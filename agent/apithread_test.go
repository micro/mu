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
	const who = "ask_keyless"
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
	const who = "ask_keyless_two"
	one, _ := Ask(AskRequest{Account: who, Client: thread.WebClient, Text: "first"})
	two, _ := Ask(AskRequest{Account: who, Client: thread.WebClient, Text: "second"})
	if one.Thread == "" || two.Thread == "" {
		t.Fatal("a call came back with no conversation")
	}
	if one.Thread == two.Thread {
		t.Error("two unrelated questions were filed on one conversation")
	}
}

// Passing the thread id back continues that conversation.
//
// This is the endpoint's whole claim over a completion API — "send thread back
// on the next call and it continues the same conversation" — and it did not
// hold. The door validated the value with thread.Get, treating it as a record
// id, then handed it to Ask as AskRequest.Thread, which is the client's own
// *key* and is resolved with thread.Open. It matched no key, so a new
// conversation was opened whose key happened to be the previous conversation's
// id: a different thread id came back and the agent had never heard of the
// first question.
//
// Found by calling the live endpoint twice, not by reading it. The first call
// looked perfect.
func TestPassingTheThreadBackContinuesIt(t *testing.T) {
	const who = "api_continue"
	first, _ := Ask(AskRequest{
		Account: who, Client: thread.WebClient, Text: "what is the weather", Trigger: "api",
	})
	if first.Thread == "" {
		t.Fatal("no conversation to continue")
	}

	// What the door does with a returned id.
	second, _ := Ask(AskRequest{
		Account: who, Client: thread.WebClient, On: first.Thread,
		Text: "and what did I just ask", Trigger: "api",
	})
	if second.Thread != first.Thread {
		t.Fatalf("the second turn opened %q instead of continuing %q",
			second.Thread, first.Thread)
	}

	// Both questions on one conversation, which is what makes the history the
	// agent is given actually contain the first one.
	msgs := thread.Messages(who, first.Thread, 10)
	var asked []string
	for _, m := range msgs {
		if m.Role != thread.RoleAgent {
			asked = append(asked, m.Text)
		}
	}
	if len(asked) < 2 {
		t.Errorf("the conversation holds %d questions, want both: %v", len(asked), asked)
	}
}
