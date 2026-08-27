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
	t.Setenv("MAIL_DOMAIN", "example.test")
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
	t.Setenv("MAIL_DOMAIN", "example.test")
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
	t.Setenv("MAIL_DOMAIN", "example.test")
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
	t.Setenv("MAIL_DOMAIN", "example.test")
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

// The address names a conversation. It does not grant permission to start one.
//
// This is the whole safety of replying from a mail client. The address is
// composed — 447700900123.sms@domain — so anybody who has seen one can write
// another. If the address alone were enough to send, knowing the pattern would
// be permission to text any number in the world from this instance's shared
// number.
func TestABridgeAddressCannotStartAConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "example.test")
	const who = "replier"

	// A number nobody here has ever spoken to.
	handled, err := Reply(who, "447700900999.sms@example.test", "buy my thing")
	if !handled {
		t.Fatal("the address was not recognised at all, so nothing checked it")
	}
	if err == nil {
		t.Fatal("a bridge address for a number with no conversation sent a message — " +
			"the address is now permission to text anybody")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	// And nothing was created on the way past. Asserting on the error alone is
	// not enough here: sms.SendOn refuses in a test environment for its own
	// reasons, so a refusal proves nothing about which rule refused.
	if thread.Find(who, thread.SMSClient, "+447700900999") != nil {
		t.Fatal("answering an address opened a conversation that did not exist, " +
			"which is the address granting permission by another route")
	}
}

// And one account cannot answer another account's conversation.
func TestABridgeAddressIsScopedToTheAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "example.test")

	said(t, "owner_a", thread.SMSClient, "+447700900123", "", "hello there")

	// Somebody else, authenticated as themselves, naming the same address.
	handled, err := Reply("owner_b", "447700900123.sms@example.test", "hello from a stranger")
	if !handled {
		t.Fatal("the address was not recognised")
	}
	if err == nil {
		t.Fatal("one account answered another account's conversation")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Errorf("the refusal is not about there being no such conversation: %v", err)
	}
	// And owner_a's conversation was not written to by owner_b.
	th := thread.Find("owner_a", thread.SMSClient, "+447700900123")
	if th == nil {
		t.Fatal("the conversation vanished")
	}
	if msgs := thread.Messages("owner_a", th.ID, 10); len(msgs) != 1 {
		t.Errorf("another account added %d messages to this conversation", len(msgs)-1)
	}
}

// A held conversation cannot be answered from a mail client either.
//
// Held means nobody has let them in. Replying is letting them in by the back
// door, and it would tell a stranger the number reaches somebody real.
func TestAHeldConversationCannotBeAnsweredByMail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "example.test")
	const who = "held_replier"

	th := said(t, who, thread.SMSClient, "+447700900777", "", "is this you")
	thread.Hold(who, th.ID)

	handled, err := Reply(who, "447700900777.sms@example.test", "who is this")
	if !handled {
		t.Fatal("the address was not recognised")
	}
	if err == nil {
		t.Fatal("a held conversation was answered, which lets a stranger in by " +
			"the back door and confirms the number reaches somebody")
	}
	// The reason, not merely a refusal. sms.SendOn refuses in a test
	// environment anyway, so "it failed" is true whether or not the held check
	// ran — and a test that cannot tell those apart is not testing the check.
	if !strings.Contains(err.Error(), "let it in") {
		t.Errorf("the refusal is not about the conversation being held: %v", err)
	}
	// Nothing reached the conversation either.
	if msgs := thread.Messages(who, th.ID, 10); len(msgs) != 1 {
		t.Errorf("the held conversation has %d messages, want the one that arrived", len(msgs))
	}
}

// An address at somebody else's domain is somebody else's mail.
func TestOnlyThisInstancesOwnDomainIsABridgeAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "example.test")

	// A positive first. Every assertion below is that an address is *not*
	// recognised, which passes just as well when nothing is ever recognised —
	// and that is exactly how this test passed while bridgeParse was reading a
	// domain setting that is not the one the mail service uses.
	if handled, _ := Reply("nobody_here", "447700900123.sms@example.test", "x"); !handled {
		t.Fatal("a real bridge address is not recognised, so the rest of this " +
			"test proves nothing")
	}

	for _, addr := range []string{
		"447700900123.sms@gmail.com",
		"henrik@example.org",
		"asim@example.test",         // an account here, not a bridge address
		"447700900123@example.test", // no channel
		"notanumber.sms@example.test",
		"447700900123.telegram@example.test", // a channel this does not speak
	} {
		if handled, _ := Reply("somebody", addr, "hello"); handled {
			t.Errorf("%q was treated as a bridge address", addr)
		}
	}
}

// The address survives the round trip, which is what makes a reply possible at
// all: a client answers what the From header said.
func TestABridgeAddressRoundTrips(t *testing.T) {
	addr := bridgeAddress("+447700900123", thread.SMSClient, "example.test")
	client, key, ok := bridgeParse(addr, "example.test")
	if !ok {
		t.Fatalf("the address this produced cannot be read back: %q", addr)
	}
	if client != thread.SMSClient || key != "+447700900123" {
		t.Errorf("round trip = %q, %q", client, key)
	}
	// And in the form a mail client actually writes it.
	if _, _, ok := bridgeParse("Somebody <"+addr+">", "example.test"); !ok {
		t.Error("an address inside angle brackets was not recognised")
	}
	// WhatsApp keeps its own channel, so a reply does not go out as a text.
	wa := bridgeAddress("+447700900123", thread.WhatsAppClient, "example.test")
	if client, _, _ := bridgeParse(wa, "example.test"); client != thread.WhatsAppClient {
		t.Errorf("a WhatsApp address reads back as %q", client)
	}
}
