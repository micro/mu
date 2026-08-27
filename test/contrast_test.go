package test

// Contrast is a pair, and a rule that changes one half of it must change both.
//
// This has now been three bugs of one shape, and the third was introduced by
// the fix for the second:
//
//  1. `button:hover` set a dark fill and no colour. Any button whose class gave
//     it a light background and dark text lost the background — a class is
//     0,0,1,0 and that selector is 0,0,1,1 — and kept the dark text, so
//     pointing at it made it dark-on-dark. On a phone :hover sticks after a
//     tap, so it was reported as "black on black when selected".
//
//  2. The fix gave `button:hover` a colour and added a list of controls that
//     should keep their own look, which took the fill away with
//     `background: none` — and said nothing about colour, so `color: #fff`
//     from the line above stayed. The same button went white-on-white.
//
// One is a mistake, three is a shape, and the shape is easy to state: taking a
// fill away without naming the text that sits on it leaves the text whatever
// the last rule said, which is a colour chosen for the fill you just removed.
//
// So: a hover rule that sets `background: none` must also set `color`. Narrow
// on purpose. It does not ask anything of rules that set a background to a real
// value — those are usually paired correctly and a broad rule here would fire
// on dozens of legitimate ones — and it does not try to compute contrast, which
// would need a colour parser and a definition of "readable" that nobody agreed.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A rule block: everything up to the brace is the selector, everything inside
// is the body.
var (
	cssRule    = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

func TestAHoverThatRemovesAFillNamesItsText(t *testing.T) {
	b, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}

	// Comments out first, or a selector is whatever prose happens to precede
	// it. The block above this very rule discusses `button:hover`, which made
	// the plain `.agent-act` rule read as a hover rule and fail.
	css := cssComment.ReplaceAllString(string(b), "\n")

	for _, m := range cssRule.FindAllStringSubmatch(css, -1) {
		selector, body := strings.TrimSpace(m[1]), m[2]
		if !strings.Contains(selector, ":hover") {
			continue
		}
		// `background: none` and `background: transparent` are the same act.
		if !removesFill(body) {
			continue
		}
		if !declares(body, "color") {
			t.Errorf("%s takes its fill away and does not say what colour the "+
				"text is, so the text keeps whatever colour was chosen for the "+
				"fill that is no longer there:\n\t{%s}",
				oneLine(selector), strings.TrimSpace(body))
		}
	}
}

// removesFill reports whether a body sets background to nothing.
func removesFill(body string) bool {
	for _, d := range strings.Split(body, ";") {
		name, value, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "background" && name != "background-color" {
			continue
		}
		switch strings.TrimSpace(value) {
		case "none", "transparent":
			return true
		}
	}
	return false
}

// declares reports whether a body sets a property — `color`, not
// `border-color`, which is why this compares the name rather than searching for
// the word.
func declares(body, property string) bool {
	for _, d := range strings.Split(body, ";") {
		if name, _, ok := strings.Cut(d, ":"); ok && strings.TrimSpace(name) == property {
			return true
		}
	}
	return false
}

// oneLine puts a multi-line selector list on one line, so the failure reads as
// a sentence rather than a paragraph.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
