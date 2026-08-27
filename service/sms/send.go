package sms

// One send path, used by the tool, the page and anything else that ever wants
// to text somebody.
//
// It is one function on purpose. Every rule this service has is a rule about
// sending, and a second path that skipped one of them would not look like a
// bug until the bill arrived — so there is nowhere to skip them from.

import (
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
)

// Send texts somebody, charging the caller for what it costs.
//
// The order matters. Everything that can refuse does so before the provider is
// called, because after that the money is spent whatever we decide; and the
// charge lands after the provider accepts, because charging for a message that
// was never sent is the one failure a caller cannot check for themselves.
func Send(owner, to, text string) (*Message, error) {
	return SendOn(ChannelSMS, owner, to, text)
}

// SendOn is Send, on a named channel.
//
// Send stays as the name of the ordinary case, because a text is what almost
// every caller wants and threading a channel through them all to say so would
// be noise. Everything either channel needs is in here — the refusals are the
// same refusals, and the four things that differ are read from the channel
// rather than branched on at each site.
func SendOn(channel Channel, owner, to, text string) (*Message, error) {
	if !channel.Known() {
		return nil, fmt.Errorf("%q is not a channel this instance sends on", string(channel))
	}
	if owner == "" {
		return nil, fmt.Errorf("sign in to send a message")
	}
	if !ConfiguredFor(channel) {
		if channel == ChannelWhatsApp {
			return nil, fmt.Errorf("this instance has no WhatsApp sender — an operator sets TWILIO_WHATSAPP_FROM")
		}
		return nil, fmt.Errorf("this instance has no number to send from")
	}

	number := e164(to)
	if number == "" {
		return nil, fmt.Errorf("%q is not a phone number in international format — use +447700900123", to)
	}
	if OursOn(channel, number) {
		return nil, fmt.Errorf("that is this instance's own number")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("there is nothing to send")
	}
	if limit := maxBodyFor(channel); len(text) > limit {
		return nil, fmt.Errorf("that is %d characters; a %s message is %d at most here",
			len(text), channel.Label(), limit)
	}
	// Segments are how a text is billed and are not a thing WhatsApp has: one
	// message costs one conversation whatever its length. Counting them anyway
	// and charging per segment would bill a paragraph five times for something
	// the provider charged once.
	segments := 1
	if channel == ChannelSMS {
		segments = Segments(text)
		if segments > maxSegments {
			return nil, fmt.Errorf("that would be sent as %d messages and charged as %d — keep it under %d characters",
				segments, segments, maxSegments*153)
		}
	}

	if OptedOut(number) {
		// Not an error the caller can work around, and it should not read like
		// one they should retry.
		return nil, fmt.Errorf("%s has asked not to receive messages from this instance", number)
	}
	// Country routing is about which of this instance's numbers can reach a
	// handset, which is a carrier question. A WhatsApp sender is one number
	// registered against a business and reaches every country the same way, so
	// the allowlist would refuse ordinary messages for a reason that does not
	// apply to them.
	if channel == ChannelSMS && !countryAllowed(number) {
		return nil, fmt.Errorf("this instance does not send to %s — ask the operator to allow that country code", number)
	}
	// And the one rule that is a rule rather than a value. See window.go.
	if channel == ChannelWhatsApp {
		if open, why := windowOpen(owner, number); !open {
			return nil, fmt.Errorf("%s", why)
		}
	}
	if KnownOnly() && !Known(owner, number) {
		return nil, fmt.Errorf("this instance only sends to numbers you already know: add %s to your "+
			"contacts first, or verify it as your own", number)
	}
	if Repeated(owner, number, text) {
		return nil, fmt.Errorf("that exact message went to %s a moment ago", number)
	}
	// After the caller's own mistakes, because this one is the operator's: a
	// country with no number here is not something the caller did wrong.
	if channel == ChannelSMS && messagingService() == "" && FromFor(number) == "" {
		return nil, fmt.Errorf("this instance has no number in that country to text %s from", number)
	}
	limit := LimitFor(owner)
	if limit == 0 {
		return nil, fmt.Errorf("this instance is not sending texts at the moment")
	}
	if n := SentToday(owner); n >= limit {
		return nil, fmt.Errorf("that is %d texts in a day, which is the limit for this account", n)
	}

	// Priced per segment, because that is how it is billed to us. A caller who
	// writes a long message pays for a long message.
	op := opFor(channel)
	cost := 0
	if quota.Metered(op) {
		ok, _, per, err := quota.CheckQuota(owner, op)
		if err != nil {
			return nil, err
		}
		cost = per * segments
		if !ok || quota.BalanceOf(owner) < cost {
			return nil, fmt.Errorf("sending that costs %d credits and there are not enough on this account", cost)
		}
	}

	id, err := sendOn(channel, number, text)
	if err != nil {
		return nil, err
	}

	for i := 0; i < segments; i++ {
		if err := chargeSend(channel, owner, map[string]interface{}{
			"to": number, "segments": segments, "channel": string(channel),
		}); err != nil {
			// The message is gone; refusing now would only hide that. Say so in
			// the log and let the caller have the message they paid for.
			logCharge(owner, err)
			break
		}
	}

	m := RecordOn(channel, owner, "out", number, text, segments)
	if id != "" {
		m.ID = id
	}
	return m, nil
}

// logCharge notes a charge that did not land after a message went out. Rare and
// worth seeing: it means somebody got a text this instance paid for and the
// account did not.
func logCharge(owner string, err error) {
	app.Log("sms", "charging %s for a sent message: %v", owner, err)
}
