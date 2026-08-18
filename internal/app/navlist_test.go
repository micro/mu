package app

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// The rail lists your mailboxes, and nothing else.
//
// One nested list, and it is the one that earns its place: the mailboxes carry
// unread counts, so the rail is telling you something rather than repeating a
// page you can open. The roster and the catalogue were tried underneath their
// own headings and turned the sidebar into a table of contents.
func TestTheRailListsYourMailboxes(t *testing.T) {
	NavMailboxes = func(string) []NavItem {
		return []NavItem{
			{Label: "All", Href: "/inbox", Badge: "3"},
			{Label: "Research", Href: "/inbox/research", Key: "research"},
		}
	}
	t.Cleanup(func() { NavMailboxes = nil })

	out := renderShell("en", "Test", "d", "", "<p>c</p>", &auth.Account{ID: "alice"}, "/inbox/research")

	for _, want := range []string{`href="/inbox/research"`, `>Research<`, `class="nav-badge">3<`} {
		if !strings.Contains(out, want) {
			t.Errorf("the rail is missing %s", want)
		}
	}
	// Where you are is marked, or the list is decoration.
	if !strings.Contains(out, `class="nav-kid on"`) {
		t.Error("the rail does not show which mailbox you are in")
	}
	// And Agents is a link to its page, not a list of agents.
	if !strings.Contains(out, `href="/agents"`) {
		t.Error("the Agents heading is missing")
	}
	if strings.Contains(out, `href="/agent/`) {
		t.Error("the roster is back in the rail")
	}
}

// A signed-out visitor has no things, so the headings stand alone rather than
// promising a list that is not there.
func TestTheRailHasNoListsForAVisitor(t *testing.T) {
	NavMailboxes = func(string) []NavItem { return []NavItem{{Label: "All", Href: "/inbox"}} }
	t.Cleanup(func() { NavMailboxes = nil })

	out := renderShell("en", "Test", "d", "", "<p>c</p>", nil, "/")
	if strings.Contains(out, "nav-kid") {
		t.Error("a signed-out visitor is shown somebody's mailboxes")
	}
	// The headings are still there — they are how you find out what they are.
	if !strings.Contains(out, `href="/inbox"`) {
		t.Error("the Inbox heading disappeared with its list")
	}
}

// Nothing to list draws nothing. A heading with an empty list under it is a
// promise the page has not kept.
func TestAnEmptyListDrawsNothing(t *testing.T) {
	NavMailboxes = func(string) []NavItem { return nil }
	t.Cleanup(func() { NavMailboxes = nil })

	out := renderShell("en", "Test", "d", "", "<p>c</p>", &auth.Account{ID: "alice"}, "/inbox")
	if strings.Contains(out, "nav-kids") {
		t.Error("an account with no mailboxes gets an empty list container")
	}
}
