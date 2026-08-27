package sms

import (
	"strings"
	"testing"
	"time"

	"mu/internal/quota"
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

// An account that is not capped is not capped.
//
// Reported as "I'm an admin, why did you limit me" — and the exemption was
// there the whole time. quota's override answers NoLimit for the operator and
// the agent, who are uncapped for the same reason they are not charged; this
// package read that NoLimit as "quota.json says nothing" and fell back to the
// instance default. One constant meaning two things, and the wrong one won, so
// an admin on their own instance was stopped at five texts by a cap written for
// strangers signing up.
func TestAnExemptAccountIsNotCapped(t *testing.T) {
	setup(t)
	t.Setenv("SMS_DAILY_LIMIT", "5")

	old := quota.LimitOverride
	defer func() { quota.LimitOverride = old }()
	quota.LimitOverride = func(account, op string) (int, bool) {
		if account == "boss" {
			return quota.NoLimit, true
		}
		return 0, false
	}

	if got := LimitOn(ChannelSMS, "boss"); got != quota.NoLimit {
		t.Errorf("an exempt account has a limit of %d — the exemption was discarded", got)
	}
	if got := LimitOn(ChannelSMS, "ordinary"); got != 5 {
		t.Errorf("an ordinary account's limit = %d, want the instance's 5", got)
	}
	// And the page says so rather than counting down from a number that does
	// not apply: NoLimit is -1, so subtracting produced "0 messages left today"
	// for the one account that never runs out.
	if got := allowance("boss"); !strings.Contains(got, "No daily limit") {
		t.Errorf("the page tells an uncapped account: %q", got)
	}
}

// The two channels have their own allowances, because they have their own
// bills. A WhatsApp reply used to spend a text.
func TestEachChannelHasItsOwnAllowance(t *testing.T) {
	setup(t)
	const who = "allowance_owner"

	RecordOn(ChannelWhatsApp, who, "out", "+447700900123", "one", 1)
	RecordOn(ChannelWhatsApp, who, "out", "+447700900123", "two", 1)
	Record(who, "out", "+447700900456", "a text", 1)

	if n := SentTodayOn(ChannelWhatsApp, who); n != 2 {
		t.Errorf("WhatsApp sent today = %d, want 2", n)
	}
	if n := SentTodayOn(ChannelSMS, who); n != 1 {
		t.Errorf("texts sent today = %d, want 1 — WhatsApp is being counted against the text allowance", n)
	}

	// And the limits come from different settings, or WHATSAPP_DAILY_LIMIT is a
	// documented setting that does nothing.
	t.Setenv("SMS_DAILY_LIMIT", "5")
	t.Setenv("WHATSAPP_DAILY_LIMIT", "40")
	if got := LimitOn(ChannelWhatsApp, who); got != 40 {
		t.Errorf("the WhatsApp limit = %d, want WHATSAPP_DAILY_LIMIT's 40", got)
	}
	if got := LimitOn(ChannelSMS, who); got != 5 {
		t.Errorf("the text limit = %d, want SMS_DAILY_LIMIT's 5", got)
	}
}
