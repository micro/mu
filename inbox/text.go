package inbox

import (
	"regexp"
	"strings"
)

func trimTo(s string, n int) string {
	s = strings.Join(strings.Fields(plain(s)), " ")
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// plain is markdown with the marks taken off, for somewhere there is no
// renderer.
//
// Not a parser. It undoes the handful of things a model actually writes —
// emphasis, code ticks, headings, bullets, numbered items, links — and leaves
// anything it does not recognise alone, because a preview that mangles an
// unusual message is worse than one that shows a stray character. A `#` in the
// middle of a sentence is a `#`; only one at the start of a line is a heading.
func plain(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		t = markLead.ReplaceAllString(t, "")   // # heading, > quote, - or 1. item
		t = markLink.ReplaceAllString(t, "$1") // [text](url) -> text
		t = markEmph.ReplaceAllString(t, "")   // ** __ * _ `
		out = append(out, strings.TrimSpace(t))
	}
	return strings.Join(out, "\n")
}

var (
	// What can only be a mark because of where it is: a heading, a block
	// quote, a bullet or a numbered item, at the start of a line. A "#" or a
	// "-" in the middle of a sentence is a "#" or a "-".
	markLead = regexp.MustCompile(`^(?:#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+)`)
	// [text](url), keeping the text. A reader wants what it said, not where it
	// went — the row is not a link to it anyway.
	markLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// The emphasis and code marks themselves.
	markEmph = regexp.MustCompile("\\*\\*|__|\\*|`")
)
