package app

// Running the place is a nav item, not a line of text on a settings page.
//
// The admin dashboard was reachable only as "Admin Dashboard →" inside the
// Settings card on /account — three clicks from anywhere, below passkeys and
// blocked users, on the page you go to in order to change your language. It is
// the page an operator opens most and it was the hardest one to reach.
//
// It sits in the bottom group with Account and Logout rather than the top one.
// The top group is what the product is, and it should look the same to
// everybody; admin is a role, and roles sit with identity.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAnAdminGetsTheLinkInTheNav(t *testing.T) {
	nav := navBottom(&auth.Account{ID: "boss", Admin: true})
	if !strings.Contains(nav, `href="/admin"`) {
		t.Fatal("an admin has no way to the dashboard from the sidebar")
	}
	if strings.Contains(nav, `id="nav-admin" href="/admin" style="display: none;"`) {
		t.Error("the admin's own link is hidden, so it needs JavaScript to appear")
	}
	// Logout stays last: it is what the group is ordered around.
	if strings.Index(nav, `id="nav-admin"`) > strings.Index(nav, `id="nav-logout"`) {
		t.Error("admin is below logout")
	}
}

// Rendered hidden for everyone else, so a page cached for an admin and served
// to the next viewer does not offer them the door. /admin checks the session
// itself; this is about not showing it.
func TestAnOrdinaryAccountIsNotShownTheDoor(t *testing.T) {
	nav := navBottom(&auth.Account{ID: "reader"})
	if !strings.Contains(nav, `id="nav-admin" href="/admin" style="display: none;"`) {
		t.Error("the admin link is not hidden for a non-admin")
	}
}

func TestSignedOutGetsNoAdminLink(t *testing.T) {
	if strings.Contains(navBottom(nil), "/admin") {
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
	if !strings.Contains(page, `href="/token"`) {
		t.Error("the account page no longer offers API credentials, so this scan " +
			"is reading the wrong function")
	}
}
