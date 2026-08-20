package app

// The rail's two levels have to actually be two levels.

import (
	"strings"
	"testing"
)

// stylesheet is mu.css with its comments taken out — they are prose about
// selectors and would otherwise answer questions asked about the selectors.
func stylesheet(t *testing.T) string {
	t.Helper()
	b, err := htmlFiles.ReadFile("html/mu.css")
	if err != nil {
		t.Fatalf("no stylesheet: %v", err)
	}
	var out strings.Builder
	rest := string(b)
	for {
		i := strings.Index(rest, "/*")
		if i < 0 {
			out.WriteString(rest)
			return out.String()
		}
		out.WriteString(rest[:i])
		j := strings.Index(rest[i:], "*/")
		if j < 0 {
			return out.String()
		}
		rest = rest[i+j+2:]
	}
}

// Nothing in the rail nests any more.
//
// It had a level under Inbox — your mailboxes, with unread counts — and the
// rules for it were written as bare `.nav-kid`, which loses to `#nav a`: an id
// beats a class whatever the source order, so not one line of them applied and
// a rail meant to read as two levels showed ten identical rows. That was fixed
// by scoping them, and then the level itself was removed: the sidebar is four
// nouns, and a fifth unfolding into a sub-list makes it a table of contents.
//
// What is held here is that the level does not come back by accident, and that
// the dead rules went with it.
func TestTheRailDoesNotNest(t *testing.T) {
	css := stylesheet(t)

	for _, gone := range []string{".nav-kid", ".nav-kids", ".nav-badge"} {
		if strings.Contains(css, gone) {
			t.Errorf("%s is still in the stylesheet, and nothing renders it", gone)
		}
	}
	// The selector that matched nothing, from when the lists were thought to
	// live outside #nav.
	if strings.Contains(css, "#nav-container > ") {
		t.Error("a rule is scoped to a direct child of #nav-container, which matches nothing")
	}
}

// ruleBody returns what is between the braces of the first rule with this
// selector.
func ruleBody(css, selector string) (string, bool) {
	i := strings.Index(css, selector)
	if i < 0 {
		return "", false
	}
	rest := css[i+len(selector):]
	end := strings.Index(rest, "}")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
