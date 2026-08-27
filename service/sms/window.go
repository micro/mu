package sms

// WhatsApp's 24-hour window.
//
// Meta lets a business send freely for 24 hours after the person last wrote to
// it. Outside that, only a template somebody has had approved in advance may be
// sent, and an ordinary message is rejected by the provider.
//
// Nothing here has templates, so this refuses and says why. That is the whole
// of the feature: a refusal a person can act on — "text them first" — instead
// of a provider error that reads as a bug in this product.
//
// It is checked rather than assumed because the failure is quiet and delayed.
// An agent told to follow something up tomorrow would compose a perfectly good
// message, spend the credits, and have it dropped by Meta with the outcome
// visible only in a provider log nobody reads.

import "time"

// window is how long a WhatsApp conversation stays open after the last inbound
// message. Meta's number, not ours.
const window = 24 * time.Hour

// openUntil is when the window on a conversation closes, and whether there is
// one at all.
//
// Read from the record rather than from a second store. Every inbound message
// is already written down with its time, so "when did they last write" is a
// question the history answers — and a separate table of window expiries is a
// copy of that which can drift from it.
func openUntil(owner, number string) (time.Time, bool) {
	number = e164(number)
	if number == "" {
		return time.Time{}, false
	}
	for _, m := range History(owner, 200) {
		if m.Direction != "in" || m.Number != number {
			continue
		}
		// On WhatsApp, and only WhatsApp. Meta's window opens when somebody
		// writes to the WhatsApp sender; a text from the same person to the
		// same instance is a different conversation on a different number and
		// says nothing about it. Counting texts here would open a window that
		// does not exist and the message would be dropped by the provider with
		// the credits already spent.
		if Channel(m.Channel) != ChannelWhatsApp {
			continue
		}
		// History is newest first, so the first inbound match is the last one
		// they sent.
		return m.At.Add(window), true
	}
	return time.Time{}, false
}

// windowOpen reports whether an ordinary WhatsApp message may be sent to this
// number now, and says why not when it may not.
func windowOpen(owner, number string) (bool, string) {
	until, ever := openUntil(owner, number)
	if !ever {
		return false, "WhatsApp only lets this instance message somebody who has messaged it first. " +
			"Ask them to send anything to " + whatsAppSender() + " and the conversation opens for 24 hours."
	}
	if time.Now().After(until) {
		return false, "that WhatsApp conversation closed " + since(until) +
			" — Meta allows a reply for 24 hours after somebody writes in, and only " +
			"pre-approved templates after that. A text will still get through."
	}
	return true, ""
}

// whatsAppSender is the number to tell somebody to write to, or a phrase that
// does not pretend to be one.
func whatsAppSender() string {
	if from := SendersFor(ChannelWhatsApp); len(from) > 0 {
		return from[0]
	}
	return "this instance's WhatsApp number"
}

// since is a rough age, for a sentence rather than for arithmetic.
func since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "less than an hour ago"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + " hours ago"
	default:
		return itoa(int(d.Hours()/24)) + " days ago"
	}
}
