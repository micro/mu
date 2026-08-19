package app

// Running the place is a nav item, not a line of text on a settings page.
//
// The admin dashboard was reachable only as "Admin Dashboard →" inside the
// Settings card on /account — three clicks from anywhere, below passkeys and
// blocked users, on the page you go to in order to change your language. It is
// the page an operator opens most and it was the hardest one to reach.
//
// It sat in the bottom group with Account and Logout, on the reasoning that
// admin is a role and roles sit with identity. That was right, and the objection
// to it was about position rather than grouping: it ended up last in the second
// group, past Usage and past whatever is pinned. Moving it under Home fixed the
// position and broke the grouping — it read as a fourth level of the product,
// between how the instance looks and what you do with it, sitting above Inbox
// for the one account that has it.
//
// It is directly under Account now, which answers both. Usage has left the group
// for a card on /account, so there is nothing between them.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAnAdminGetsTheLinkInTheNav(t *testing.T) {
	nav := navAdmin(&auth.Account{ID: "boss", Admin: true})
	if !strings.Contains(nav, `href="/admin"`) {
		t.Fatal("an admin has no way to the dashboard from the sidebar")
	}
	if strings.Contains(nav, "display: none") {
		t.Error("the admin's own link is hidden, so it needs JavaScript to appear")
	}
	// And it is drawn where it now claims to be: directly under Account, with
	// nothing between them and Support after.
	page := renderWithLang("t", "d", "", "en", &auth.Account{ID: "boss", Admin: true})
	acct, adm, support := strings.Index(page, `id="nav-account"`), strings.Index(page, `id="nav-admin"`),
		strings.Index(page, `id="nav-support"`)
	if adm < acct || adm > support {
		t.Errorf("admin is not between Account and Support (account %d, admin %d, support %d)",
			acct, adm, support)
	}
	// Directly under: nothing else is drawn in the gap. This is what the last
	// arrangement lost — Admin was in the right group and at the wrong end of it.
	if between := page[acct:adm]; strings.Count(between, "<a ") != 1 {
		t.Errorf("something has been added between Account and Admin: %q", between)
	}
}

// Nothing at all for anybody else, rather than a hidden link JavaScript removes:
// the nav is rendered per viewer, so there is no cached page to defend against.
func TestAnOrdinaryAccountIsNotShownTheDoor(t *testing.T) {
	if nav := navAdmin(&auth.Account{ID: "reader"}); nav != "" {
		t.Errorf("a non-admin is offered the admin dashboard: %s", nav)
	}
	if strings.Contains(navBottom(&auth.Account{ID: "reader"}), "/admin") {
		t.Error("the bottom group still carries an admin link")
	}
}

func TestSignedOutGetsNoAdminLink(t *testing.T) {
	if navAdmin(nil) != "" || strings.Contains(navBottom(nil), "/admin") {
		t.Error("a signed-out visitor is offered the admin dashboard")
	}
}

// And the account page carries nothing that only an operator can use.
//
// /account had two: "Admin Dashboard →" and "Invites →", both drawn only for
// admins, both duplicating an entry on the admin dashboard itself. They were
// there because there was nowhere else to put them — the sidebar had no Admin
// link, so operator errands accumulated on the settings page, three clicks from
// anywhere and mixed in with changing your language.
//
// Read from the source rather than a rendered page, because the failure this
// guards against is somebody adding the next one, and it should be caught where
// it is written.
func TestTheAccountPageHoldsNoOperatorErrands(t *testing.T) {
	src, err := os.ReadFile("../../account/pages.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "func Account(")
	if i < 0 {
		t.Fatal("the account handler has moved; this test needs repointing")
	}
	j := strings.Index(body[i:], "\nfunc ")
	if j < 0 {
		t.Fatal("could not find the end of the account handler")
	}
	page := body[i : i+j]

	if strings.Contains(page, `href="/admin`) {
		t.Error("the account page links into /admin — operator errands belong " +
			"behind the Admin entry in the sidebar, not on the page you open to " +
			"change your language")
	}
	// Not vacuous: the handler is still building the page it is supposed to.
	// The links are app.Links pairs rather than hand-written anchors now, so
	// this looks for the path. What it is guarding is unchanged: that the scan
	// above ran over a handler that is actually building the account page.
	if !strings.Contains(page, `"/token"`) {
		t.Error("the account page no longer offers API credentials, so this scan " +
			"is reading the wrong function")
	}
}
