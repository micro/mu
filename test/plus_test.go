package test

// No button wears a plus.
//
// Some did and some did not: "+ Write" on the blog, "+ New agent" on the
// roster, "+ New app" under apps, "+ New" as the shared default — and then
// "New" in the inbox, plain, doing the same job. Reported as exactly that
// inconsistency, and the resolution is the shorter one: the plus adds nothing
// a verb does not already say, and half the product had already dropped it.
//
// The zoom control on the map keeps its "+". That is a plus sign as the whole
// label, paired with a minus, meaning the arithmetic rather than decorating a
// word — which is why this matches a plus *followed by a word* rather than a
// plus anywhere.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A plus at the start of a label, and only there.
//
// Two shapes, both with the plus *inside* the string: straight after a ">",
// which is markup — ">+ Write", ">+tag" — and straight after an opening quote
// followed by a space, which is a label argument — ActionLink(href, "+ New").
//
// Neither can be Go concatenation. "a" + b puts a space or a quote between the
// two, and "a"+b puts the plus after a closing quote with no space and a
// lowercase identifier after it; both were matched by the looser version of
// this and swamped the real hits.
var plusLabel = regexp.MustCompile(`>\+\s*[A-Za-z]|"\+ [A-Za-z]`)

func TestNoLabelStartsWithAPlus(t *testing.T) {
	for _, file := range goFiles(t) {
		rel := strings.TrimPrefix(filepath.ToSlash(file), "../")
		// IMAP's "+" is the continuation response from RFC 3501 — the protocol
		// asking the client to go on. It is a wire token, not something a
		// person reads.
		if rel == "service/mail/imap.go" {
			continue
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			// Comments describe what used to be there, including the labels
			// this removed. A gravestone is not a control.
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
				continue
			}
			if !plusLabel.MatchString(line) {
				continue
			}
			t.Errorf("%s draws a label starting with a plus:\n    %s",
				rel, strings.TrimSpace(line))
		}
	}
}
