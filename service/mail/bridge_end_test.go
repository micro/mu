package mail

import (
	"strings"
	"testing"
	"time"
)

// The bridge reaches INBOX, and renders as a message a client can read.
//
// Not the bridge in isolation: the point is what a mail client fetching INBOX
// actually receives, which is imapFolder plus imapRender.
func TestBridgedConversationsAreInTheInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "example.test")

	old := Bridged
	defer func() { Bridged = old }()
	Bridged = func(accountID string) []*Message {
		if accountID != "reader" {
			return nil
		}
		return []*Message{{
			ID: "r1", ToID: "reader",
			From: "+447700900123", FromID: "447700900123.sms@example.test",
			Subject: "+447700900123", Body: "running ten minutes late",
			MessageID: "<r1@example.test>", CreatedAt: time.Now(),
		}}
	}

	msgs, ok := imapFolder("reader", "INBOX")
	if !ok {
		t.Fatal("INBOX is not a folder")
	}
	found := false
	for _, m := range msgs {
		if m.ID == "r1" {
			found = true
		}
	}
	if !found {
		t.Fatal("a bridged conversation is not in INBOX, so a pull-refresh in a " +
			"mail client shows nothing new")
	}

	// And it renders as mail: a sender a client can show, a subject, a date.
	wire := string(imapRender(msgs[len(msgs)-1]))
	for _, want := range []string{
		"From: +447700900123 <447700900123.sms@example.test>",
		"Subject: +447700900123",
		"running ten minutes late",
		"Message-ID: <r1@example.test>",
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("the message a client receives is missing %q:\n%s", want, wire)
		}
	}

	// Somebody else's bridged conversation is not in this account's INBOX.
	other, _ := imapFolder("stranger", "INBOX")
	for _, m := range other {
		if m.ID == "r1" {
			t.Fatal("one account's conversation is in another account's INBOX")
		}
	}
}
