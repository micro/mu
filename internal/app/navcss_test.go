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

// The rules for a child item must outrank the rules for a nav link.
//
// They were written as bare `.nav-kid`, which loses to `#nav a` — an id beats a
// class, whatever the source order — so not one line of them applied. Your
// mailboxes and your agents rendered as full-weight top-level entries, and a
// rail written to read as two levels showed ten identical rows. They were also
// prefixed `#nav-container > `, a selector that matches nothing at all, since
// the lists live inside #nav.
//
// This is exactly the kind of thing that is invisible in review and obvious on
// the screen, so it is asserted rather than remembered.
func TestTheRailsChildItemsOutrankItsLinks(t *testing.T) {
	css := stylesheet(t)

	for _, sel := range []string{"#nav .nav-kids {", "#nav a.nav-kid {", "#nav a.nav-kid.on {"} {
		if !strings.Contains(css, sel) {
			t.Errorf("%s is not in the stylesheet, so #nav a wins and the rail is flat", sel)
		}
	}
	// The selector that matched nothing.
	if strings.Contains(css, "#nav-container > ") {
		t.Error("a rule is scoped to a direct child of #nav-container; the rail's lists are inside #nav")
	}
	// And a bare .nav-kid rule would be one that never applies.
	for _, dead := range []string{"\n.nav-kid {", "\n.nav-kid.on {", "\n.nav-kids {"} {
		if strings.Contains(css, dead) {
			t.Errorf("%q is unscoped, so #nav a outranks it", strings.TrimSpace(dead))
		}
	}
}

// A group is separated from the next one. On a phone the rail is the whole
// screen and there is nothing else to give it structure.
func TestTheRailSeparatesOneGroupFromTheNext(t *testing.T) {
	css := stylesheet(t)
	block, ok := ruleBody(css, "#nav .nav-kids {")
	if !ok {
		t.Fatal("no rule for the rail's groups")
	}
	for _, want := range []string{"border-left", "border-bottom"} {
		if !strings.Contains(block, want) {
			t.Errorf("the group has no %s, so it does not read as a group", want)
		}
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
