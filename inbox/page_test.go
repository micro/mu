package inbox

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// An alias is a mailbox.
//
// asim+research@ goes to the research agent, so what arrives there is that
// agent's mail and not a slice of yours — the same shape as a mailbox per
// client, arrived at from the other direction. The tag rides on the message
// already, so nothing new is stored to make this true.
func TestEachAliasIsItsOwnMailbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "boxes-owner"
	deliver(t, who, "a@example.com", "About the research", "research")
	deliver(t, who, "b@example.com", "Something for the briefer", "briefer")
	deliver(t, who, "c@example.com", "Just for you", "")

	all := listBody(t, "/inbox", who)
	for _, want := range []string{"About the research", "Something for the briefer", "Just for you"} {
		if !strings.Contains(all, want) {
			t.Errorf("the whole inbox is missing %q", want)
		}
	}

	// The switcher offers every box that has something in it.
	for _, want := range []string{`href="/inbox/research"`, `href="/inbox/briefer"`, `href="/inbox"`} {
		if !strings.Contains(all, want) {
			t.Errorf("no way to reach %s", want)
		}
	}

	one := listBody(t, "/inbox/research", who)
	if !strings.Contains(one, "About the research") {
		t.Error("the research box does not hold its own mail")
	}
	for _, other := range []string{"Something for the briefer", "Just for you"} {
		if strings.Contains(one, other) {
			t.Errorf("the research box also shows %q", other)
		}
	}
}

// A switcher with one destination is a control that cannot do anything.
func TestNoSwitcherWhenThereIsOnlyTheOneMailbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "one-box-owner"
	deliver(t, who, "a@example.com", "Ordinary mail", "")

	// The markup, not the stylesheet — inboxCSS always carries the rule.
	if body := listBody(t, "/inbox", who); strings.Contains(body, `<div class="ib-boxes">`) {
		t.Error("an account with no aliases is offered a mailbox switcher")
	}
}

// An empty box says so, rather than saying the inbox is empty — the narrower
// fact is the true one, and the address is already on the page above it.
func TestAnEmptyBoxSaysWhichBoxIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "empty-box-owner"
	deliver(t, who, "a@example.com", "About the research", "research")

	body := listBody(t, "/inbox/briefer", who)
	if !strings.Contains(body, "briefer") {
		t.Errorf("an empty box does not name itself:\n%s", body)
	}
	if strings.Contains(body, "About the research") {
		t.Error("an empty box is showing another box's mail")
	}
}

// listBody renders the inbox for an account without going through auth.
func listBody(t *testing.T, path, accountID string) string {
	t.Helper()
	w := httptest.NewRecorder()
	list(w, httptest.NewRequest("GET", path, nil), accountID, boxOf(httptest.NewRequest("GET", path, nil)))
	return w.Body.String()
}
