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
	// And it is drawn in the menu with the other destinations: after Account,
	// before Log out.
	//
	// This used to assert "directly under Account, nothing in the gap, Support
	// after". Both halves have gone. Support was removed from the product and
	// this menu went on linking to a route that no longer exists; and Saved and
	// Tokens moved here off /account, where they had been filed under a section
	// called "Settings" on the settings page. So there is something in the gap
	// now, by design.
	//
	// What survives is the claim worth holding: admin is a destination in the
	// menu rather than a line of text three clicks down a settings page, and it
	// is above Log out, which is last because it ends the session.
	page := renderWithLang("t", "d", "", "en", &auth.Account{ID: "boss", Admin: true})
	acct, adm, out := strings.Index(page, `id="nav-account"`), strings.Index(page, `id="nav-admin"`),
		strings.Index(page, `id="nav-logout"`)
	if acct < 0 || adm < 0 || out < 0 {
		t.Fatalf("the account menu is missing an item (account %d, admin %d, logout %d)", acct, adm, out)
	}
	if adm < acct || adm > out {
		t.Errorf("admin is not between Account and Log out (account %d, admin %d, logout %d)",
			acct, adm, out)
	}
}

// The bottom of the rail is who you are and the way out, and nothing else.
//
// It used to be a menu holding everything the account owns — Account, Profile,
// Wallet, Tokens, Admin — behind a disclosure under your own name. Those are
// destinations and they are in navMain now; what is left here is the pair that
// is not a destination.
//
// The dead-link checks stay. Saved pointed at /user, deleted along with the
// feed controls it held, and Support pointed at /support after that page and
// its mailbox were removed — both survived here as links to nothing, which is
// what this half of the test exists to catch.
func TestTheBottomIsWhoYouAreAndTheWayOut(t *testing.T) {
	bottom := navBottom(&auth.Account{ID: "someone"})

	if !strings.Contains(bottom, "Signed in as") || !strings.Contains(bottom, "@someone") {
		t.Errorf("the rail does not say which account this is: %q", bottom)
	}
	if !strings.Contains(bottom, `id="nav-logout" href="/logout"`) {
		t.Error("no way to log out")
	}
	// The destinations moved up. Any of them back here is a decision.
	for _, moved := range []string{"/account", "/@someone", "/token", "/wallet"} {
		if strings.Contains(bottom, `href="`+moved+`"`) {
			t.Errorf("%s is in the bottom group again rather than the rail", moved)
		}
	}
	// And the links to pages that no longer exist.
	for _, gone := range []string{"/user", "/about", "/support"} {
		if strings.Contains(bottom, `href="`+gone+`"`) {
			t.Errorf("the rail links to %s, which is not served", gone)
		}
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
	//
	// This used to anchor on "/token", which has left for the account menu along
	// with Saved and About — so the anchor went with the thing it was anchoring
	// to, and the guard reported the account page as missing rather than the
	// test as needing repointing. Then it anchored on BalanceCard, "the last
	// thing that would ever move off it", and the money moved to /wallet.
	//
	// Twice now, which says something about the anchor rather than the page: a
	// card is a product decision and product decisions move. The profile is
	// what /account cannot stop drawing without ceasing to be the account page.
	if !strings.Contains(page, `app.Section("Profile"`) {
		t.Error("the account page no longer draws the profile, so this scan " +
			"is reading the wrong function")
	}
}
