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

// The menu holds what is yours, and Log out is last.
//
// Saved, Tokens and About were cards on /account — a credential list, three
// piles of things you kept, and the footer in a card, all filed next to the
// balance because two sections there were named "Settings" and "About Mu" and a
// name that broad absorbs anything.
//
// Two of the three have since left the menu as well, and for the rule this test
// is named after rather than despite it. Saved pointed at /user, which was
// deleted with the feed controls it held, so it was a dead link of exactly the
// kind the /support check below exists to catch. About is not yours — it is a
// page about us, it is in the footer, and the menu under your own name is the
// wrong place for it. Profile took the slot: your own page, which until then
// could only be reached by typing it.
func TestTheAccountMenuHoldsWhatIsYours(t *testing.T) {
	menu := navBottom(&auth.Account{ID: "someone"})

	for _, want := range []struct{ id, href string }{
		{"nav-account", "/account"},
		{"nav-profile", "/@someone"},
		{"nav-token", "/token"},
		{"nav-logout", "/logout"},
	} {
		if !strings.Contains(menu, `id="`+want.id+`" href="`+want.href+`"`) {
			t.Errorf("the account menu has no %s pointing at %s", want.id, want.href)
		}
	}

	// The two that left, named so that putting either back is a decision.
	for _, gone := range []string{"/user", "/about"} {
		if strings.Contains(menu, `href="`+gone+`"`) {
			t.Errorf("the account menu links to %s again", gone)
		}
	}

	// Log out ends the session, so nothing is drawn after it — a control below
	// it reads as being on a page that has already finished. That is exactly
	// what happened to the notifications card on /account.
	if out := strings.Index(menu, `id="nav-logout"`); strings.Count(menu[out:], "<a ") != 1 {
		t.Errorf("something is drawn after Log out:\n%s", menu[out:])
	}

	// Support was removed from the product — the page, the mailbox, the
	// settings — and this menu went on linking to /support, which does not
	// route. A dead link in the menu is worse than no link: it is the one place
	// somebody looks when something has gone wrong.
	if strings.Contains(menu, "/support") {
		t.Error("the account menu still links to /support, which no longer exists")
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
