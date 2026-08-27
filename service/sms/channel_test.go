package sms

import (
	"strings"
	"testing"
	"time"
)

// The wire prefix goes on at the edge and nowhere else.
//
// Twilio labels both ends "whatsapp:+447700900123". Everywhere above the
// provider call a number is a number — internal/phone.Normalise refuses a
// prefixed one outright, since one was silently turned into +44447700900123, a
// real number belonging to a stranger, and filed against their phone.
func TestTheChannelIsReadOffTheWireAndTakenOff(t *testing.T) {
	setup(t)

	c, n := ChannelOf("whatsapp:+447700900123")
	if c != ChannelWhatsApp || n != "+447700900123" {
		t.Errorf("ChannelOf(whatsapp:…) = %q, %q", c, n)
	}
	if c, n := ChannelOf("+447700900123"); c != ChannelSMS || n != "+447700900123" {
		t.Errorf("ChannelOf(bare) = %q, %q", c, n)
	}
	// And back on again, in the request that actually goes out — not merely in
	// the helper that puts it there. A WhatsApp message sent without its prefix
	// is delivered as a text, from a number the recipient does not recognise,
	// and charged as a text.
	t.Setenv("TWILIO_WHATSAPP_FROM", "+447700900000")
	form, err := sendForm(ChannelWhatsApp, "+447700900123", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got := form.Get("To"); got != "whatsapp:+447700900123" {
		t.Errorf("To on the wire = %q, want the whatsapp: prefix", got)
	}
	if got := form.Get("From"); got != "whatsapp:+447700900000" {
		t.Errorf("From on the wire = %q, want the whatsapp: prefix", got)
	}
	// A WhatsApp sender is registered with Meta against a business, so routing
	// through a country-matching sender pool would send from a number that is
	// not a WhatsApp sender at all.
	t.Setenv("TWILIO_MESSAGING_SERVICE_SID", "MG00000000000000000000000000000000")
	if form, err := sendForm(ChannelWhatsApp, "+447700900123", "hello"); err != nil {
		t.Fatal(err)
	} else if form.Get("MessagingServiceSid") != "" {
		t.Error("a WhatsApp message was routed through the SMS sender pool")
	}

	// And a text is not given a prefix.
	if form, err := sendForm(ChannelSMS, "+447700900123", "hello"); err != nil {
		t.Fatal(err)
	} else if got := form.Get("To"); got != "+447700900123" {
		t.Errorf("a text was given a channel prefix: %q", got)
	}
}

// A channel nobody has heard of is refused, not treated as a text.
//
// Sending a WhatsApp message as an SMS is not a graceful fallback: it arrives
// on the other person's phone as a second conversation from a number they do
// not recognise, and it is charged differently.
func TestAnUnknownChannelIsRefused(t *testing.T) {
	setup(t)
	if _, err := SendOn(Channel("telegram"), "somebody", "+447700900123", "hello"); err == nil {
		t.Fatal("a channel this instance does not speak was accepted")
	}
}

// WhatsApp is not rationed by segment, because it is not billed by segment.
//
// A text is 160 characters a segment and this instance pays for three. One
// WhatsApp message costs one 24-hour conversation whatever its length, so
// applying the segment limit would refuse an ordinary paragraph that costs
// exactly what "ok" costs.
func TestWhatsAppIsNotMeasuredInSegments(t *testing.T) {
	setup(t)
	if maxBodyFor(ChannelWhatsApp) <= maxBodyFor(ChannelSMS) {
		t.Error("a WhatsApp message is capped at a text's length, which rations " +
			"something that is not being billed")
	}
	if opFor(ChannelWhatsApp) == opFor(ChannelSMS) {
		t.Error("both channels charge the same operation, so an operator cannot " +
			"price a conversation differently from a segment")
	}
}

// The 24-hour window is checked here rather than left to the provider.
//
// Meta accepts an ordinary message only within 24 hours of the person last
// writing in. Outside it, only pre-approved templates, and there are none here.
// Left unchecked, an agent told to follow something up tomorrow would compose a
// good message, spend the credits, and have it dropped with the outcome visible
// only in a provider log.
func TestTheWhatsAppWindowIsCheckedBeforeSpendingAnything(t *testing.T) {
	setup(t)
	t.Setenv("TWILIO_WHATSAPP_FROM", "+447700900000")
	const who = "window_owner"
	const them = "+447700900321"

	// Nobody has written in, so there is no window at all.
	if open, why := windowOpen(who, them); open {
		t.Error("a WhatsApp conversation was open with somebody who has never written in")
	} else if !strings.Contains(why, "messaged it first") {
		t.Errorf("the reason does not say what to do about it: %q", why)
	}

	// They write in: the window opens.
	RecordOn(ChannelWhatsApp, who, "in", them, "hello", 1)
	if open, why := windowOpen(who, them); !open {
		t.Errorf("the window did not open after they wrote in: %q", why)
	}

	// And it is the last inbound message that decides, not the first.
	if until, ever := openUntil(who, them); !ever || time.Until(until) > window {
		t.Error("the window is not measured from when they last wrote")
	}
}

// A text does not open a WhatsApp window.
//
// Meta's window opens when somebody writes to the WhatsApp sender. A text from
// the same person is a different conversation on a different number and says
// nothing about it — counting one would open a window that does not exist, and
// the message would be dropped by the provider with the credits already spent.
func TestATextDoesNotOpenAWhatsAppWindow(t *testing.T) {
	setup(t)
	t.Setenv("TWILIO_WHATSAPP_FROM", "+447700900000")
	const who = "crosstalk_owner"
	const them = "+447700900654"

	Record(who, "in", them, "hello by text", 1)

	if open, _ := windowOpen(who, them); open {
		t.Error("a text opened a WhatsApp conversation — the next WhatsApp " +
			"message would be charged for and then dropped by Meta")
	}
}

// The record says which channel carried a message, because the reply has to go
// back the same way.
func TestTheRecordRemembersTheChannel(t *testing.T) {
	setup(t)
	const who = "channel_owner"

	RecordOn(ChannelWhatsApp, who, "in", "+447700900444", "hi", 1)
	Record(who, "in", "+447700900555", "hi", 1)

	var wa, sms int
	for _, m := range History(who, 50) {
		switch m.Channel {
		case string(ChannelWhatsApp):
			wa++
		case string(ChannelSMS):
			sms++
		}
	}
	if wa != 1 || sms != 1 {
		t.Errorf("the record has %d WhatsApp and %d SMS messages, want one of each", wa, sms)
	}
}
