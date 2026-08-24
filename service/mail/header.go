package mail

// Putting a value into a header without letting it become a header.
//
// A header ends at a newline, so a value carrying one does not go into a
// header — it *adds* one. Subject, To, Cc and Reply-To were written straight
// into the message with fmt.Sprintf and no check, which means a subject of
//
//	Invoice\r\nBcc: somebody@else.example
//
// sends a copy to somebody the sender never sees named. Every one of those
// values is reachable from something a stranger wrote: the agent composes mail
// from web pages and inbound messages it was asked to act on, a display name
// is whatever an account typed into its profile, and /inbox is a text box.
//
// The rule already existed one directory over. imapHeaderValue does exactly
// this for mail on the way *out to a client*, with a comment saying why — "a
// raw UTF-8 byte in a header is not a header" — and the send path, which is
// the one talking to other people's servers, never had it. Same shape as the
// delivery branch: one door had the rule and the rest had a copy without it.
//
// Nothing here is about the body. That goes out through net/smtp's Data
// writer, which is a textproto.DotWriter and dot-stuffs, so a body containing
// a lone "." on a line cannot end the message early.

import (
	"mime"
	"strings"
)

// headerText is a value for a header that holds prose: a subject, a display
// name.
//
// Folded to one line, then encoded whenever it is not plain ASCII — a raw
// UTF-8 byte in a header is not a header, and an accented name is the ordinary
// case rather than an exotic one.
func headerText(v string) string {
	v = headerLine(v)
	for i := 0; i < len(v); i++ {
		if v[i] > 126 || v[i] < 32 {
			return mime.QEncoding.Encode("utf-8", v)
		}
	}
	return v
}

// headerAddr is a value for a header that holds addresses or message ids: To,
// Cc, Reply-To, In-Reply-To, References.
//
// Folded to one line and nothing else. These have a syntax of their own that
// RFC 2047 encoding would break — an encoded-word is not an address — and the
// recipients a message actually reaches are the ones named in RCPT TO, not the
// ones in this header. What matters here is that the value cannot end the
// header it is in.
func headerAddr(v string) string { return headerLine(v) }

// headerLine folds a value onto one line.
//
// Every control character goes, not only CR and LF: a bare CR or a lone LF
// each end a line somewhere, mail servers disagree about which, and a NUL
// truncates the message at whichever end of the wire is written in C. Replaced
// with a space rather than removed, so the value stays readable and two words
// do not silently become one.
func headerLine(v string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, strings.TrimSpace(v))
}

// headerFrom is a From value: an address, with a display name in front of it
// when there is one.
//
// One function because there were three copies and each built the string with
// Sprintf, so a display name of `Bob\r\nBcc: ...` forged a header on any of
// them.
func headerFrom(displayName, address string) string {
	address = headerAddr(address)
	if name := headerText(displayName); name != "" {
		return name + " <" + address + ">"
	}
	return address
}
