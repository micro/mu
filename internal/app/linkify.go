package app

// Making a URL somebody typed into a link somebody can click.
//
// Reported as "links are not clickable in any of the emails I got", and it was
// two faults with one symptom.
//
// The first is here: /inbox renders what a person wrote with html.EscapeString
// and nothing else, so a URL in a message is text. The agent's own answers went
// through the markdown renderer and came out linked, so the two halves of the
// same conversation behaved differently — which is the tell that this was
// missing rather than declined.
//
// The second is in the mail itself and is fixed where the mail is written: a
// notice from this instance carried a bare URL in a text/plain body and a
// relative href, and a relative href in an email points at the mail client.
// Links in mail have to be absolute. See admin/alert.go and agent/mail/welcome.go.
//
// # Escaped in, HTML out
//
// Linkify takes text that has *already* been escaped and returns HTML. That
// ordering is the only safe one: escaping afterwards would eat the anchors it
// just wrote, and linkifying raw text would let a URL carrying markup through
// unescaped. service/social already had it this way; this is that code with
// one behaviour added and a name that says what it takes.

import (
	"strings"
)

// linkStops are the characters a URL cannot contain, so they end it.
//
// The trailing punctuation cases are handled separately: a URL at the end of a
// sentence is followed by a full stop, which is not part of it.
const linkStops = " \t\n\r\"'<>()[]{}|\\^`"

// Linkify turns URLs in already-escaped text into anchors.
//
// http:// and https:// only. Not www., and not bare domains: "see mu.test for
// details" is a sentence, and a linkifier that guesses turns prose into links
// that go nowhere. Somebody who wants a link writes a scheme.
func Linkify(escaped string) string {
	var b strings.Builder
	for i := 0; i < len(escaped); {
		rest := escaped[i:]
		if !strings.HasPrefix(rest, "http://") && !strings.HasPrefix(rest, "https://") {
			b.WriteByte(escaped[i])
			i++
			continue
		}
		// Not mid-word: "xhttps://" is not a link, and neither is the tail of
		// an href we are being handed by mistake.
		if i > 0 && !isBoundary(escaped[i-1]) {
			b.WriteByte(escaped[i])
			i++
			continue
		}

		end := i
		for end < len(escaped) && !strings.ContainsRune(linkStops, rune(escaped[end])) {
			end++
		}
		url := escaped[i:end]
		// Trailing punctuation belongs to the sentence, not the address.
		for len(url) > 0 && strings.ContainsRune(".,;:!?", rune(url[len(url)-1])) {
			url = url[:len(url)-1]
		}
		// An escaped entity at the end — "&amp;" — means the escaper ran, and
		// half of one would produce a broken href.
		if url == "" || strings.HasSuffix(url, "&") {
			b.WriteByte(escaped[i])
			i++
			continue
		}

		b.WriteString(`<a href="` + url + `" rel="noopener noreferrer">` + url + `</a>`)
		i += len(url)
	}
	return b.String()
}

// isBoundary reports whether c can sit immediately before a URL.
func isBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '(', '[', '<', '"', '\'', '>', ';':
		return true
	}
	return false
}
