package whatsapp

import (
	"strings"

	"mu/service/sms"
	"testing"
	"time"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	t.Setenv("TWILIO_AUTH_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("TWILIO_WHATSAPP_FROM", "+14155238886")
}

// The rule that shapes this service is not ours. WhatsApp allows a free-form
// message only within 24 hours of the other person's last one, and outside that
// window a business may send nothing but a template approved in advance. There
// are none here, so the honest form is: reply to people, do not start
// conversations.
func TestYouMayReplyAndNotInitiate(t *testing.T) {
	setup(t)
	const me, them = "acct-1", "+447700900123"

	if Open(me, them) {
		t.Fatal("a conversation nobody has started is open")
	}
	if _, err := Send(me, them, "hello"); err == nil ||
		!strings.Contains(err.Error(), "24 hours") {
		t.Errorf("sending to somebody who has not written: %v", err)
	}

	Record(me, "in", them, "hi there")
	if !Open(me, them) {
		t.Error("somebody wrote and the window did not open")
	}
	if until := OpenUntil(me, them); time.Until(until) > 24*time.Hour ||
		time.Until(until) < 23*time.Hour {
		t.Errorf("the window closes in %v, want about 24 hours", time.Until(until))
	}
}

// Billed by the conversation rather than the message, so charged the same way:
// the message that opens a window costs and the rest of the thread does not,
// because we are not billed for those either.
func TestOnlyTheFirstMessageInAWindowIsCharged(t *testing.T) {
	setup(t)
	const me, them = "acct-1", "+447700900123"

	Record(me, "in", them, "hi")
	if !Fresh(me, them) {
		t.Error("the first reply should open a billable conversation")
	}

	Record(me, "out", them, "hello back")
	if Fresh(me, them) {
		t.Error("a second reply in the same window was treated as a new conversation")
	}
	if !Fresh(me, "+447700900999") {
		t.Error("a different person is a different conversation")
	}
}

// A WhatsApp conversation always starts with the other person, so "whoever
// wrote to them last from here" does not exist the first time. A number already
// proved to belong to somebody is the answer that does.
func TestInboundGoesToWhoeverOwnsTheNumber(t *testing.T) {
	setup(t)
	const them = "+447700900123"

	// With no claim and no conversation it goes to whoever runs the instance,
	// because a message nobody ever sees is the worst of the outcomes.
	if got := OwnerOf(them); got != sms.Fallback() {
		t.Errorf("OwnerOf = %q for a stranger, want the fallback %q", got, sms.Fallback())
	}

	Record("acct-2", "out", them, "hello")
	if got := OwnerOf(them); got != "acct-2" {
		t.Errorf("OwnerOf = %q, want the last conversation", got)
	}
}

// A number is a number whichever medium it carries, so the addresses have to
// agree with the ones next door.
func TestAddressing(t *testing.T) {
	setup(t)
	if got := address("+447700900123"); got != "whatsapp:+447700900123" {
		t.Errorf("address = %q", got)
	}
	for _, in := range []string{"whatsapp:+44 7700 900123", "+447700900123", "whatsapp:+44-7700-900123"} {
		if got := number(in); got != "+447700900123" {
			t.Errorf("number(%q) = %q", in, got)
		}
	}
}

func TestSendRefusesBeforeItCosts(t *testing.T) {
	setup(t)
	const me = "acct-1"
	for _, c := range []struct{ name, to, text, want string }{
		{"not a number", "hello", "hi", "international format"},
		{"our own number", "+14155238886", "hi", "own WhatsApp number"},
		{"nothing to say", "+447700900123", "  ", "nothing to send"},
	} {
		if _, err := Send(me, c.to, c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want something about %q", c.name, err, c.want)
		}
	}
}
