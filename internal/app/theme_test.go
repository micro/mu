package app

// The stylesheet says which themes this product has, and it has one.
//
// "Black text on black background", then "we keep coming across this issue".
// Both true, and the second is the interesting one: every previous report of
// this shape was fixed on the class it was noticed on, and the next one turned
// up somewhere else a week later. mu.css records five such rounds already —
// "found and patched five separate times, on five different classes, because
// each fix was to the class rather than to this".
//
// The cause is not any class. There is no dark theme in this product: no
// prefers-color-scheme block, no [data-theme], no second palette. A browser
// that darkens the page anyway repaints backgrounds and leaves author colours
// declared !important alone, so white-on-dark becomes white-on-light and
// black-on-light becomes black-on-dark, with every rule in the file still
// satisfied.
//
// color-scheme is how a page says what it supports. Declared light, a browser
// does not invent a dark one.
//
// So two things are asserted, and the second is what keeps this honest: the
// declaration is present, *and* it still matches reality. The day somebody
// writes a real dark palette, this fails and tells them to come and change the
// declaration rather than leaving the page pinned to light for everybody.

import (
	"strings"
	"testing"
)

func TestTheStylesheetDeclaresItsTheme(t *testing.T) {
	b, err := htmlFiles.ReadFile("html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)

	if !strings.Contains(css, "color-scheme: light") {
		t.Error("mu.css does not declare color-scheme, so a browser is free to " +
			"darken a page built entirely of light-theme colours — which is what " +
			"produces black text on a black background, class after class")
	}

	// And it is still true. A dark palette arriving without the declaration
	// changing means half the product is dark and the browser has been told
	// the whole of it is light.
	for _, dark := range []string{"prefers-color-scheme: dark", `[data-theme="dark"]`} {
		if strings.Contains(css, dark) {
			t.Errorf("mu.css now has %q but still declares color-scheme: light — "+
				"the declaration has to name every theme the page supports, or "+
				"the browser will not switch to the one that was just written", dark)
		}
	}
}
