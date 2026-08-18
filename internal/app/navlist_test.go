package app

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// The rail lists your things, not just the pages they live on.
//
// Inbox, Agents, Tools and Services were four links — a set of pages. A client
// puts your mailboxes and your agents in the rail, so you move between them
// rather than opening a page to find out what they are.
func TestTheRailListsYourMailboxesAndYourAgents(t *testing.T) {
	NavMailboxes = func(string) []NavItem {
		return []NavItem{{Label: "All", Href: "/inbox"}, {Label: "Research", Href: "/inbox/research", Key: "research"}}
	}
	NavAgents = func(string) []NavItem {
		return []NavItem{{Label: "Micro", Href: "/agent/micro", Key: "micro"}}
	}
	t.Cleanup(func() { NavMailboxes, NavAgents = nil, nil })

	out := renderShell("en", "Test", "d", "", "<p>c</p>", &auth.Account{ID: "alice"}, "/inbox/research")

	for _, want := range []string{`href="/inbox/research"`, `href="/agent/micro"`, `>Research<`, `>Micro<`} {
		if !strings.Contains(out, want) {
			t.Errorf("the rail is missing %s", want)
		}
	}
	// Where you are is marked, or the list is decoration.
	if !strings.Contains(out, `class="nav-kid on"`) {
		t.Error("the rail does not show which mailbox you are in")
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
