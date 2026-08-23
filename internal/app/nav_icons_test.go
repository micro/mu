package app

// Every icon the chrome asks for is one this binary has.
//
// The Billing entry shipped pointing at /credits.svg, which does not exist and
// never did — the name was picked by reading the line that referenced it, which
// was the same line. Nothing failed: the page renders, the link works, and the
// only sign is a broken image in the menu, which is exactly the kind of thing
// you see once, assume is a cache, and stop noticing.
//
// The whole nav is embedded under html/, so "does this file exist" is a
// question the test binary can answer without a server, a session or a network.

import (
	"regexp"
	"strings"
	"testing"

	"mu/internal/auth"
)

// asset finds the src of every <img> the chrome draws, minus the cache-busting
// query. They are all served from the root off the same embedded directory —
// see the handler that maps /x.svg to html/x.svg.
var asset = regexp.MustCompile(`src="/([A-Za-z0-9._-]+\.(?:svg|png))`)

func TestEveryNavIconExists(t *testing.T) {
	// Both states of the menu, because they draw different entries: signed out
	// is one link, signed in is the whole list, and an admin gets one more.
	pages := []string{
		renderWithLang("t", "d", "", "en", nil),
		renderWithLang("t", "d", "", "en", &auth.Account{ID: "someone"}),
		renderWithLang("t", "d", "", "en", &auth.Account{ID: "boss", Admin: true}),
	}

	seen := map[string]bool{}
	for _, page := range pages {
		for _, m := range asset.FindAllStringSubmatch(page, -1) {
			seen[m[1]] = true
		}
	}
	if len(seen) == 0 {
		t.Fatal("no icons found in the rendered chrome, so this test is asserting nothing")
	}

	for name := range seen {
		if _, err := htmlFiles.ReadFile("html/" + name); err != nil {
			t.Errorf("the chrome asks for /%s and this binary has no html/%s — "+
				"a broken image in the sidebar, which nothing else reports", name, name)
		}
	}
}

// And the menu under your own name draws an icon for every entry.
//
// A missing src is the other half of the same failure and looks different: the
// row collapses to its label rather than showing a broken image, so it reads
// as a deliberately plain entry rather than a mistake.
func TestEveryAccountMenuEntryHasAnIcon(t *testing.T) {
	menu := navBottom(&auth.Account{ID: "someone", Admin: true})
	for _, row := range strings.Split(menu, "<a ")[1:] {
		id := ""
		if i := strings.Index(row, `id="`); i >= 0 {
			id = row[i+4:]
			id = id[:strings.Index(id, `"`)]
		}
		if !strings.Contains(row[:strings.Index(row, "</a>")], "<img ") {
			t.Errorf("the %s entry has no icon", id)
		}
	}
}
