package client

// The agent as an entry in your address book.
//
// # Why a file and not a page
//
// A page listing a phone number, a WhatsApp number, an address and a URL is
// four things to copy out by hand, and nobody does it. The point of an
// assistant you can text is that texting it is the easy thing — and it is only
// easy once it is in your contacts, because that is where a phone keeps the
// people you talk to. Until then every message starts with going to find the
// number.
//
// A vCard is how a phone accepts a new contact from anywhere: open it and it
// offers to save. One tap, and the agent is in the same list as everybody else
// you write to, with every way of reaching it under one name. That is the whole
// feature, and it is the difference between "there is a number on a page
// somewhere" and "I texted it from the bus".
//
// # Only what works
//
// Built from All(), so it carries exactly the channels this instance actually
// has: a number only where one is configured, mail only where the domain is
// real. A contact card with a dead number in it is worse than no contact card,
// because a phone will keep the dead number until somebody notices.
//
// # The version
//
// 3.0, not 4.0. iOS and Android both read 3.0 without complaint and have for
// fifteen years; 4.0 support is patchier than its age suggests, and there is
// nothing here that 3.0 cannot say.

import (
	"strconv"
	"strings"

	"mu/service/sms"
)

// Savable reports whether there is anything here a phone can hold.
//
// A number or an address. Without one, the card is a name and a URL — which is
// a bookmark, not a contact, and a button offering to save it promises more
// than it does. Web and CLI are always in All() and neither is something an
// address book has a field for.
func Savable() bool {
	for _, c := range All() {
		switch c.ID {
		case "sms", "whatsapp", "mail":
			return true
		}
	}
	return false
}

// VCard is the contact card for this instance's agent, as a phone reads one.
//
// name is what it will be saved as — the agent's display name, passed in rather
// than resolved here because this package does not know about agents: it knows
// about the ways in. See home.ContactHandler, which has both.
func VCard(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Assistant"
	}

	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\n")
	b.WriteString("VERSION:3.0\r\n")
	// N is the structured name and FN the one that is displayed. Both, because
	// a card with only FN sorts unpredictably in some address books — and the
	// structured form of a single-word name is that word as the given name.
	b.WriteString("N:;" + escapeVCard(name) + ";;;\r\n")
	b.WriteString("FN:" + escapeVCard(name) + "\r\n")

	// The organisation is the instance, which is what somebody with two of
	// these needs to tell them apart: the same agent name on two instances is
	// two different assistants with two different memories.
	if host := Host(); host != "" {
		b.WriteString("ORG:" + escapeVCard(host) + "\r\n")
	}

	// One entry per way in, and the labels are what a phone shows beneath the
	// number. item<N>.X-ABLabel is Apple's convention and Android reads it too;
	// without it a second number is just "other".
	item := 0
	for _, c := range All() {
		switch c.ID {
		case "sms":
			item++
			b.WriteString(labelled(item, "TEL;TYPE=CELL:"+c.Address, "Text"))
		case "whatsapp":
			// The same number on both channels is one entry, not two. A phone
			// that holds a number twice offers it twice in every menu, and the
			// second one is indistinguishable from a mistake.
			if same := sms.From(); same != "" && sameNumber(same, c.Address) {
				continue
			}
			item++
			b.WriteString(labelled(item, "TEL;TYPE=CELL:"+c.Address, "WhatsApp"))
		case "mail":
			item++
			b.WriteString(labelled(item, "EMAIL;TYPE=INTERNET:"+c.Address, "Write to it"))
		}
	}

	if host := Host(); host != "" {
		b.WriteString("URL:https://" + escapeVCard(host) + "\r\n")
	}
	// What it is, for somebody looking at the card a month later wondering who
	// this is. The note is the only free text a phone will show.
	b.WriteString("NOTE:" + escapeVCard(note(name)) + "\r\n")
	b.WriteString("END:VCARD\r\n")
	return b.String()
}

// note is the sentence saved with the card.
func note(name string) string {
	n := name + " answers on every number and address on this card, and the same "
	n += "conversation carries across them."
	if host := Host(); host != "" {
		n += " On the web: https://" + host
	}
	return n
}

// labelled writes one property under an item group so the label travels with
// it. A property with no group takes the phone's own idea of what to call it.
func labelled(item int, property, label string) string {
	prefix := "item" + strconv.Itoa(item) + "."
	return prefix + property + "\r\n" + prefix + "X-ABLabel:" + escapeVCard(label) + "\r\n"
}

// sameNumber compares two numbers as a phone would: by their digits.
//
// One instance writes +44 7700 900000 and another +447700900000, and they are
// the same phone. Comparing the strings makes the SMS and WhatsApp entries look
// different whenever they were typed differently, which puts the same number in
// somebody's address book twice.
func sameNumber(a, b string) bool { return digits(a) == digits(b) }

func digits(s string) string {
	var out strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// escapeVCard escapes the characters that are structure in a vCard line.
//
// Backslash first, or every escape this adds afterwards is escaped again. The
// values here are an instance's own configuration rather than a stranger's
// input, but a comma in an operator's chosen agent name would silently split a
// field into two — which is the sort of thing that shows up as half a name in
// somebody's phone.
func escapeVCard(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
