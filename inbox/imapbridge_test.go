package inbox

import (
	"strings"
	"testing"

	"mu/internal/thread"
	"mu/service/mail"
)

// The whole record shows up in a mail client, not just the mail.
//
// The requirement in one sentence: pull-refresh the IMAP inbox in Gmail and get
// all the threads. IMAP delivers nothing — it is a reader over a message store
// — so the store worth pointing it at is the one every channel writes to.
func TestEveryConversationIsThereForAMailClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "example.test")
	const who = "bridge_reader"

	said(t, who, thread.SMSClient, "+447700900123", "", "running ten minutes late")
	said(t, who, thread.WhatsAppClient, "+447700900456", "", "dinner at eight?")
	said(t, who, thread.ChatClient, "henrik@example.org", "", "did you see the deploy")

	got := Bridge(who)
	if len(got) < 3 {
		t.Fatalf("the bridge produced %d messages for three conversations", len(got))
	}

	body := ""
	for _, m := range got {
		body += m.Body + "\n"
	}
	for _, want := range []string{"running ten minutes late", "dinner at eight?", "did you see the deploy"} {
		if !strings.Contains(body, want) {
			t.Errorf("a conversation is missing from what a mail client would see: %q", want)
		}
	}
}

// Mail is not bridged, or every message arrives twice.
//
// The record holds mail too — agent/mail writes it above the delivery — so
// handing it back here would show each email once as an envelope out of the
// mail store and once as prose out of the record.
func TestMailIsNotBridged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "example.test")
	const who = "no_double"

	said(t, who, mailClient, "henrik@example.org", "", "the invoice is attached")
	said(t, who, thread.SMSClient, "+447700900123", "", "a text")

	for _, m := range Bridge(who) {
		if strings.Contains(m.Body, "the invoice is attached") {
			t.Fatal("mail was bridged out of the record as well as served from " +
				"the mail store, so every email appears twice")
		}
	}
}

// A held conversation stays out. Being held is the whole point: somebody nobody
// here has heard of has not been let in, and pushing it to a phone through
// Gmail is exactly what that is meant to prevent.
func TestAHeldConversationIsNotPushedToAMailClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "example.test")
	const who = "held_reader"

	th := said(t, who, thread.SMSClient, "+447700900999", "", "cheap watches")
	thread.Hold(who, th.ID)

	for _, m := range Bridge(who) {
		if strings.Contains(m.Body, "cheap watches") {
			t.Fatal("a held conversation reached a mail client")
		}
	}
}

// What somebody else said arrives; what you and your agent said is sent.
//
// A client that showed the agent's own answers as new mail would ring for every
// reply it gave. service/mail files on FromID, so the two sides have to be
// marked correctly or the whole conversation lands in one folder.
func TestYourOwnRepliesAreSentAndTheirsArrive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "example.test")
	const who = "sides"

	th := thread.Open(who, thread.SMSClient, "+447700900123")
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		From: "+447700900123", Text: "are you free"})
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		Role: thread.RoleAgent, Text: "checking your calendar now"})

	var arrived, sent int
	for _, m := range Bridge(who) {
		switch {
		case strings.Contains(m.Body, "are you free"):
			arrived++
			if m.FromID == who {
				t.Error("a message from somebody else is marked as sent by the account")
			}
			// And it has an address, or a mail client cannot show a sender.
			if !strings.Contains(m.FromID, "@") {
				t.Errorf("no address for the sender: %q", m.FromID)
			}
		case strings.Contains(m.Body, "checking your calendar"):
			sent++
			if m.FromID != who {
				t.Error("the agent's own reply is not marked as sent by the account, " +
					"so a client would ring for it as new mail")
			}
		}
	}
	if arrived != 1 || sent != 1 {
		t.Errorf("%d arrived and %d sent, want one of each", arrived, sent)
	}
}

// A number becomes an address a client can show, and the ids are stable.
func TestAPhoneNumberBecomesAnAddress(t *testing.T) {
	got := bridgeAddress("+44 7700 900123", thread.SMSClient, "example.test")
	if got != "447700900123.sms@example.test" {
		t.Errorf("bridgeAddress = %q", got)
	}
	// A local part may not begin with a plus: that is this instance's own tag
	// separator, and an address like +44…@ would read as an agent box.
	if strings.HasPrefix(got, "+") {
		t.Error("the address begins with the tag separator")
	}
	// An address is left alone.
	if got := bridgeAddress("henrik@example.org", thread.ChatClient, "example.test"); got != "henrik@example.org" {
		t.Errorf("an address was rewritten: %q", got)
	}
	// Stable, because a UID once given means that message forever.
	if bridgeID("abc") != bridgeID("abc") {
		t.Error("the same message gets a different id each time, so UIDs churn")
	}
	if bridgeID("abc") == bridgeID("abd") {
		t.Error("two messages share an id")
	}
}

// The bridge is what mail asks for, and mail asks for it.
func TestTheBridgeIsWiredToTheMailService(t *testing.T) {
	// Not the boot wiring — that is internal/server — but the shape it needs:
	// a function of the right type, exported, that mail can be handed.
	var _ func(string) []*mail.Message = Bridge
}
