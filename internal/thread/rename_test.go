package thread

// The conversation survives being claimed.
//
// Somebody writes to agent@ from an address nobody knows, is answered, and is
// later invited to sign up and keep it. That invitation is only true if the
// record moves with the account — and an account id is the key of `owned`, the
// Account on every Thread and the Account on every Message, so it has to move in
// three places at once.

import (
	"testing"
)

func TestARenamedAccountKeepsItsConversations(t *testing.T) {
	const old, claimed = "rename-guest-abc123", "rename-claimed-asim"

	th := Open(old, "mail", "rename-1")
	if th == nil {
		t.Fatal("no conversation")
	}
	Add(Message{Thread: th.ID, Account: old, Text: "what is the weather",
		From: "asim@example.com"})
	Add(Message{Thread: th.ID, Account: old, Text: "Cloudy, 14°C.", Role: RoleAgent})

	Rename(old, claimed)

	// Nothing is left behind under the old name.
	if got := List(old, 10); len(got) != 0 {
		t.Errorf("%d conversations still filed under the old id", len(got))
	}
	// And everything is there under the new one.
	threads := List(claimed, 10)
	if len(threads) != 1 {
		t.Fatalf("%d conversations under the claimed account, want 1 — somebody "+
			"invited to keep their conversation signed up and found nothing", len(threads))
	}
	if threads[0].Account != claimed {
		t.Errorf("the conversation still says it belongs to %q", threads[0].Account)
	}
	msgs := Messages(claimed, th.ID, 10)
	if len(msgs) != 2 {
		t.Fatalf("%d messages after the rename, want 2", len(msgs))
	}
	for _, m := range msgs {
		if m.Account != claimed {
			t.Errorf("message %q still filed under %q", m.Text, m.Account)
		}
	}
	// Reachable by id, which is what every page does.
	if Get(claimed, th.ID) == nil {
		t.Error("the conversation cannot be opened under the claimed account")
	}
}

// Two people's conversations are never merged.
//
// A new id that somehow already has a record is the one case where doing
// nothing is better than proceeding: joining two strangers' conversations is
// worse than a rename that did not happen.
func TestARenameNeverMergesTwoAccounts(t *testing.T) {
	const from, into = "guest-def456", "somebody"

	mine := Open(from, "mail", "rename-2")
	theirs := Open(into, "mail", "rename-3")
	if mine == nil || theirs == nil {
		t.Fatal("no conversations")
	}
	Add(Message{Thread: mine.ID, Account: from, Text: "mine"})
	Add(Message{Thread: theirs.ID, Account: into, Text: "theirs"})

	Rename(from, into)

	if got := List(into, 10); len(got) != 1 {
		t.Errorf("%d conversations under %q after a colliding rename — two people's "+
			"records were merged", len(got), into)
	}
	if got := List(from, 10); len(got) != 1 {
		t.Errorf("the source account lost its record to a rename that should not "+
			"have happened (%d conversations)", len(got))
	}
}
