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
