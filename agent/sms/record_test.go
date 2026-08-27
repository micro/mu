package sms

import (
	"testing"

	"mu/internal/thread"
)

// A text from somebody unknown is recorded and held, not dropped.
//
// It used to be dropped with a log line, because the only path into the record
// was the side effect of an agent answering — and the agent is exactly what a
// stranger must not be able to start. So the one safe thing to do with an
// unsolicited message was also the thing that made it vanish.
func TestATextFromAStrangerIsHeldRatherThanLost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "held_owner"

	record(texted{Owner: who, From: "+447700900999", Text: "is this the plumber"}, false)

	// In the record.
	waiting := thread.HeldFor(who, 10)
	if len(waiting) != 1 {
		t.Fatalf("%d conversations held, want 1 — a stranger's text was lost", len(waiting))
	}
	if waiting[0].Key != "+447700900999" {
		t.Errorf("the held conversation is keyed %q", waiting[0].Key)
	}
	if msgs := thread.Messages(who, waiting[0].ID, 5); len(msgs) != 1 {
		t.Errorf("the held conversation has %d messages, want 1 — what they said was lost", len(msgs))
	}

	// And not in the inbox, which is the entire point of holding it.
	for _, got := range thread.List(who, 50) {
		if got.ID == waiting[0].ID {
			t.Error("a held conversation is in the list — a stranger put a line " +
				"in front of somebody without being let in")
		}
	}
}

// A text from a number the account knows goes straight in.
func TestATextFromSomebodyKnownIsNotHeld(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "known_owner"

	record(texted{Owner: who, From: "+447700900123", Text: "running late"}, true)

	if n := len(thread.HeldFor(who, 10)); n != 0 {
		t.Errorf("%d conversations held, want 0 — a known sender was held", n)
	}
	if n := len(thread.List(who, 50)); n != 1 {
		t.Errorf("%d conversations in the list, want 1", n)
	}
}

// Letting one in puts it in the inbox and leaves everything else alone.
func TestLettingOneInPutsItInTheInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "release_owner"

	record(texted{Owner: who, From: "+447700900888", Text: "delivery is outside"}, false)
	held := thread.HeldFor(who, 10)
	if len(held) != 1 {
		t.Fatalf("%d held, want 1", len(held))
	}

	thread.Release(who, held[0].ID)

	if n := len(thread.HeldFor(who, 10)); n != 0 {
		t.Errorf("%d still held after being let in", n)
	}
	list := thread.List(who, 50)
	if len(list) != 1 || list[0].ID != held[0].ID {
		t.Fatalf("a released conversation is not in the inbox: %d rows", len(list))
	}
	if msgs := thread.Messages(who, list[0].ID, 5); len(msgs) != 1 {
		t.Error("releasing a conversation lost what was said in it")
	}
}

// A second text from the same stranger joins the conversation already held,
// rather than starting a second one — and does not quietly un-hold it.
func TestASecondTextFromTheSameStrangerStaysHeld(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "twice_owner"

	record(texted{Owner: who, From: "+447700900777", Text: "first"}, false)
	record(texted{Owner: who, From: "+447700900777", Text: "second"}, false)

	held := thread.HeldFor(who, 10)
	if len(held) != 1 {
		t.Fatalf("%d conversations held, want 1 — the number is the conversation", len(held))
	}
	if msgs := thread.Messages(who, held[0].ID, 10); len(msgs) != 2 {
		t.Errorf("the held conversation has %d messages, want 2", len(msgs))
	}
}
