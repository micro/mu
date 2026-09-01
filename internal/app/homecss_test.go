package app

// Home's column rules have to be able to override each other.
//
// The layout is three media queries deep: one column on a phone, a rail beside
// one column of cards at 1024, a rail beside two at 1400. Each of those wins
// over the last only because it comes later in the file — they are all the same
// selector, so the cascade breaks the tie on source order.
//
// That holds until one of them is scoped and the others are not. :not() takes
// the specificity of its argument, so .home-main:not(.full) #home carries two
// classes where .home-main #home carries one, and a lower breakpoint written
// that way wins from anywhere in the file. That is what happened when the
// signed-out layout was added: the 1024 rule grew a :not(.full), the 1400 rule
// did not, and Home stopped being three columns at any width — with both rules
// present, both correct-looking, and the media queries doing exactly what they
// said.
//
// Nothing about that is visible in a diff of either rule on its own, which is
// why it is pinned here rather than left to be noticed on a wide screen.

import (
	"regexp"
	"strings"
	"testing"
)

// A selector that is overridden at a later breakpoint must be written the same
// way at every breakpoint, or the later one cannot win.
func TestHomesBreakpointsCanOverrideEachOther(t *testing.T) {
	css := stylesheet(t)

	// Every selector in the file that ends at #home and is scoped by
	// .home-main. These are the ones that fight: one per breakpoint, each
	// meant to beat the one above it.
	sel := regexp.MustCompile(`\.home-main[^,{}\n]*#home\s*\{`)

	found := map[string]int{}
	for _, m := range sel.FindAllString(css, -1) {
		found[strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m), "{"))]++
	}
	if len(found) == 0 {
		t.Fatal("no .home-main #home rules at all — Home's column layout has moved " +
			"and this test is looking at the wrong thing")
	}
	if len(found) > 1 {
		var forms []string
		for s, n := range found {
			forms = append(forms, s+" ("+plural(n)+")")
		}
		t.Errorf("Home's breakpoints set flex-direction on #home through %d different "+
			"selectors:\n    %s\n"+
			"They override each other by source order, which only works while they "+
			"are equally specific. :not() counts as its argument, so one of these "+
			"beats the others from whichever breakpoint it is written at and the "+
			"rest never apply.",
			len(found), strings.Join(forms, "\n    "))
	}
}

// And the signed-out column really does span both tracks, since that is the
// rule the scoping above exists to make room for.
func TestTheRaillessColumnSpansBothTracks(t *testing.T) {
	css := stylesheet(t)

	i := strings.Index(css, ".home-main.full")
	if i < 0 {
		t.Fatal("no .home-main.full rule — a Home with nothing in its rail sits in " +
			"the second grid track with an empty 320px one beside it")
	}
	body := css[i:]
	if j := strings.Index(body, "}"); j > 0 {
		body = body[:j]
	}
	if !strings.Contains(body, "grid-column") {
		t.Error(".home-main.full does not set grid-column, so it stays in one track")
	}
}

func plural(n int) string {
	if n == 1 {
		return "1 rule"
	}
	return string(rune('0'+n)) + " rules"
}
