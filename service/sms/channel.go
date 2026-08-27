package sms

// Which carrier a message rides on: a text, or WhatsApp.
//
// # Why this is a value and not a second service
//
// There was a service/whatsapp. It was 2,100 lines and it was deleted, because
// almost all of it was this one retyped: opt-out, duplicate suppression, the
// known-sender rule, the daily cap, the record, the webhook. The header on
// twilio.go already says where the seam is — internal/twilio holds the
// credentials and the POST, and "whether a message may be sent, to whom and at
// what price is here, because WhatsApp answers those differently and shares
// every line of the rest."
//
// It cannot be a second service either way. TestServicesDoNotImportEachOther
// has an empty allowlist, so a service/whatsapp could not call one line of
// this — it would have to own a second opt-out list and a second spam policy,
// and two spam policies that disagree is worse than either.
//
// So: a value threaded through, and four differences that are values too.
//
//	                sms                     whatsapp
//	  wire          +447700900123           whatsapp:+447700900123
//	  sender        TWILIO_FROM             TWILIO_WHATSAPP_FROM
//	  length        160 a segment, 3 max    4096, one message
//	  price         per segment             per 24-hour conversation
//
// The fifth is a rule rather than a value and is handled in send.go: outside a
// 24-hour window from the last inbound message, WhatsApp accepts only templates
// somebody has had approved. Nothing here has templates, so it refuses and says
// why.

import (
	"strings"

	"mu/internal/quota"
	"mu/internal/settings"
)

// Channel is how a message travels.
type Channel string

const (
	// ChannelSMS is an ordinary text. The default everywhere, so the zero value
	// of a Channel is the behaviour every existing caller already had.
	ChannelSMS Channel = ""

	// ChannelWhatsApp is WhatsApp over the same provider.
	ChannelWhatsApp Channel = "whatsapp"
)

// wirePrefix is what Twilio puts in front of a number to say which channel it
// is. Only ever applied at the edge — in deliver on the way out and in the
// webhook on the way in — so that a number anywhere else in this product is a
// number. See internal/phone.Normalise, which refuses one of these outright
// after a prefixed sender was silently turned into a stranger's number.
const wirePrefix = "whatsapp:"

// ChannelOf reads the channel off a wire address, and returns the bare number.
//
// Twilio says "whatsapp:+447700900123" on both From and To. Anything without a
// prefix is a text.
func ChannelOf(addr string) (Channel, string) {
	addr = strings.TrimSpace(addr)
	if rest := strings.TrimPrefix(addr, wirePrefix); rest != addr {
		return ChannelWhatsApp, strings.TrimSpace(rest)
	}
	return ChannelSMS, addr
}

// wire is a number as the provider wants it for this channel.
func (c Channel) wire(number string) string {
	if c == ChannelWhatsApp {
		return wirePrefix + number
	}
	return number
}

// Known reports whether this is a channel this service speaks. Anything else is
// refused rather than treated as a text — a caller who asked for a channel that
// does not exist did not mean "send it however".
func (c Channel) Known() bool {
	return c == ChannelSMS || c == ChannelWhatsApp
}

// Label is what to call it on a page.
func (c Channel) Label() string {
	if c == ChannelWhatsApp {
		return "WhatsApp"
	}
	return "SMS"
}

// maxBodyFor is how long one message may be.
//
// A text is 160 characters a segment and this instance pays for at most three.
// WhatsApp is one message up to 4096 whatever its length, so the segment
// arithmetic is not a smaller number here, it is the wrong question — applying
// it would refuse an ordinary paragraph that costs exactly the same as "ok".
func maxBodyFor(c Channel) int {
	if c == ChannelWhatsApp {
		return maxWhatsAppBody
	}
	return maxBody
}

// SendersFor is the numbers this instance can send from on a channel.
//
// WhatsApp has its own sender and only ever one: a WhatsApp sender is a number
// registered with Meta against a business, not a pool you route by country, so
// the country matching that TWILIO_FROM needs does not apply.
func SendersFor(c Channel) []string {
	if c != ChannelWhatsApp {
		return Senders()
	}
	var out []string
	for _, part := range strings.Split(settings.Get("TWILIO_WHATSAPP_FROM"), ",") {
		if n := e164(part); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// ConfiguredFor reports whether this instance can send on a channel.
func ConfiguredFor(c Channel) bool {
	if user, pass := credentials(); user == "" || pass == "" {
		return false
	}
	if c == ChannelWhatsApp {
		return len(SendersFor(ChannelWhatsApp)) > 0
	}
	return Configured()
}

// maxWhatsAppBody is one WhatsApp message. The protocol's own limit, because
// there is no per-segment cost to ration against.
const maxWhatsAppBody = 4096

// OursOn reports whether a number is one this instance sends from on a channel.
func OursOn(c Channel, number string) bool {
	n := e164(number)
	for _, s := range SendersFor(c) {
		if s == n {
			return true
		}
	}
	return false
}

// chargeSend debits one unit of whatever this channel is billed in.
//
// The branch is here, naming both operations, rather than a quota.Charge called
// with an operation picked by opFor. Charging through a variable works and is
// invisible: test/charging_test.go scans for the constant beside the call, and
// an operation nothing appears to charge is reported as free in practice. It
// was right to complain — a price nobody can find the charge site for is a
// price that quietly stops being taken.
func chargeSend(c Channel, owner string, meta map[string]interface{}) error {
	if c == ChannelWhatsApp {
		return quota.Charge(owner, quota.OpWhatsAppSend, meta)
	}
	return quota.Charge(owner, quota.OpSMSSend, meta)
}

// opFor is what a message on this channel is charged as.
//
// A second operation rather than a multiplier on the first, because the two are
// billed on different units — a text per 160-character segment, WhatsApp per
// 24-hour conversation — and an operator setting one price has to be able to
// set the other. See internal/quota.
func opFor(c Channel) string {
	if c == ChannelWhatsApp {
		return quota.OpWhatsAppSend
	}
	return quota.OpSMSSend
}
