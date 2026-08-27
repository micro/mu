package mail

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// Deleting an account deletes its mail, and the mail stays deleted.
//
// DeleteInbox removed the account's entry in `inboxes` and nothing else. That
// map is derived — rebuildInboxes reconstructs all of it from `messages` — so
// the next delivery to anybody on the instance put the deleted account's inbox
// back, out of mail that had never been removed. Everything they had ever sent
// or received was still on disk, and still readable by whoever signed up with
// the name next.
func TestDeletingAnAccountDeletesItsMail(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	going := account(t, "delgoing")
	staying := account(t, "delstaying")

	// Their mailbox: something that arrived, and something they asked their
	// agent.
	arrived, err := Deliver(Outgoing{FromID: staying, Display: "Staying", To: going + "@example.test",
		Subject: "for the leaver", Body: "hello"})
	if err != nil {
		t.Fatalf("deliver to them: %v", err)
	}
	asked, err := Deliver(Outgoing{FromID: going, Display: "Going", To: "agent@example.test",
		Subject: "a question", Body: "what is the weather"})
	if err != nil {
		t.Fatalf("their question: %v", err)
	}
	// And something they sent to somebody who is staying, which is that
	// person's correspondence rather than theirs.
	sentOn, err := Deliver(Outgoing{FromID: going, Display: "Going", To: staying + "@example.test",
		Subject: "goodbye", Body: "so long"})
	if err != nil {
		t.Fatalf("their outgoing mail: %v", err)
	}

	// In that order, because that is the order it happens in: DeleteAccount
	// removes the account and then runs the hooks, so by the time this one runs
	// there is no account behind the address any more. Getting it the other way
	// round in a test would prove something that never occurs.
	if err := auth.DeleteAccount(going); err != nil {
		t.Fatalf("delete the account: %v", err)
	}
	DeleteInbox(going)

	if m := storedByMessageID(arrived); m != nil {
		t.Error("mail sent to the deleted account is still on disk")
	}
	if m := storedByMessageID(asked); m != nil {
		t.Error("what the deleted account asked its agent is still on disk")
	}
	if m := storedByMessageID(sentOn); m == nil {
		t.Error("mail they had sent to somebody still here was taken out of that " +
			"person's mailbox — deleting an account does not unsend")
	}

	// The half that made this survive: the index is rebuilt from the messages,
	// so a delivery to anybody puts back whatever was not actually deleted.
	if _, err := Deliver(Outgoing{FromID: staying, Display: "Staying", To: "agent@example.test",
		Subject: "unrelated", Body: "something else"}); err != nil {
		t.Fatalf("later delivery: %v", err)
	}

	mutex.RLock()
	box := inboxes[going]
	mutex.RUnlock()
	if box != nil && len(box.Threads) > 0 {
		t.Errorf("the deleted account's inbox came back with %d conversations in "+
			"it after the next delivery", len(box.Threads))
	}
	if listed := ListMessages(going, 50); len(listed) > 0 {
		t.Errorf("mail_inbox still lists %d messages for a deleted account", len(listed))
	}
}

// Being the sender is one question, asked in one place.
//
// FromID holds an account id on one path and an address on another, and ten
// places compared it to an account id directly. Every one of them was asking
// "did this account write it" and every one was wrong for the address form —
// including the two that gate deletion, so a message you had sent could not be
// deleted at all: "message not found".
func TestASenderIsRecognisedInEitherForm(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "formsender")

	for _, tc := range []struct {
		name string
		msg  *Message
		want bool
	}{
		{"the account id, as the sent-copy path stores it",
			&Message{FromID: "formsender"}, true},
		{"the address, as local delivery stores it",
			&Message{FromID: "formsender@example.test"}, true},
		{"the address with a tag on it",
			&Message{FromID: "formsender+research@example.test"}, true},
		{"the address in the wrong case",
			&Message{FromID: "FormSender@EXAMPLE.TEST"}, true},
		{"somebody else here",
			&Message{FromID: "someoneelse@example.test"}, false},
		// The collision that makes a naive local-part check wrong: a stranger
		// whose address happens to share the local part.
		{"a stranger sharing the local part",
			&Message{FromID: "formsender@somewhere-else.test"}, false},
		{"nobody at all",
			&Message{FromID: ""}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentBy(tc.msg, me); got != tc.want {
				t.Errorf("sentBy(%q) = %v, want %v", tc.msg.FromID, got, tc.want)
			}
		})
	}
}

// And the consequence, on the path a person actually takes: deleting a message
// you sent.
func TestYouCanDeleteAMessageYouSent(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "deleter")
	account(t, "deletee")

	id, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "deletee@example.test",
		Subject: "sent", Body: "something"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	m := storedByMessageID(id)
	if m == nil {
		t.Fatal("nothing was filed")
	}
	if err := DeleteMessage(m.ID, me); err != nil {
		t.Fatalf("deleting a message I sent: %v — the sender check compares "+
			"FromID to an account id, and local delivery stores an address", err)
	}
	if storedByMessageID(id) != nil {
		t.Error("the message is still there")
	}
}

// Nothing in the package compares FromID to an account id by hand any more.
//
// Ten places did. Finding them was the fix; the check is here so the eleventh
// is a failing test rather than a report.
func TestNothingAsksWhoSentItByHand(t *testing.T) {
	for _, name := range goFilesHere(t) {
		if strings.HasSuffix(name, "_test.go") || name == "mail.go" {
			continue
		}
		src := readSource(t, name)
		for _, bad := range []string{
			"FromID == acc.ID", "FromID != acc.ID",
			"FromID == userID", "FromID != userID",
			"FromID == accountID", "FromID != accountID",
		} {
			if strings.Contains(src, bad) {
				t.Errorf("%s compares %s directly; FromID is an account id on one "+
					"path and an address on another, so this is false for half the "+
					"mail on the instance. Use sentBy", name, bad)
			}
		}
	}
}
