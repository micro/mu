package thread

import "testing"

// One arrival, described twice, is one message.
//
// Mail has two callers who both see the same delivery: one writes down that it
// arrived, the other runs the agent that answers it. Neither can be made to
// depend on the other having gone first — they run on separate goroutines — so
// the store settles it on the client's own identifier for the message.
func TestTheSameMessageRecordedTwiceIsRecordedOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "dedupe-account"
	th := Open(who, "mail", "<root@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}

	first := Add(Message{Thread: th.ID, Account: who, Text: "about the invoice",
		Ref: "<msg1@example.com>"})
	second := Add(Message{Thread: th.ID, Account: who, Text: "about the invoice",
		Ref: "<msg1@example.com>"})

	if first == "" {
		t.Fatal("the first was not recorded")
	}
	if second != first {
		t.Errorf("the second recording got id %q, want the first's %q", second, first)
	}
	if msgs := Messages(who, th.ID, 0); len(msgs) != 1 {
		t.Fatalf("the conversation holds %d messages, want 1", len(msgs))
	}
}

// Only Ref carries this. Two people saying the same thing is two messages, and
// nothing without an identifier of its own is anybody else's to deduplicate.
func TestMessagesWithNoRefAreNeverCollapsed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "dedupe-noref"
	th := Open(who, WebClient, "chat")
	if th == nil {
		t.Fatal("no conversation")
	}
	Add(Message{Thread: th.ID, Account: who, Text: "ok"})
	Add(Message{Thread: th.ID, Account: who, Text: "ok"})

	if msgs := Messages(who, th.ID, 0); len(msgs) != 2 {
		t.Errorf("saying the same thing twice left %d messages, want 2", len(msgs))
	}
}
