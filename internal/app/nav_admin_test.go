package app

// Running the place is a door, and not one of the four the product is.
//
// The admin dashboard was reachable only as "Admin Dashboard →" inside the
// Settings card on /account — three clicks from anywhere, below passkeys and
// blocked users, on the page you go to in order to change your language. It is
// the page an operator opens most and it was the hardest one to reach.
//
// Then the bottom group with Account and Log out, on the reasoning that admin
// is a role and roles sit with identity; then second in the rail under Home,
// on the reasoning that the foot of a list is the wrong place for something
// opened several times a day. That was right about the frequency and wrong
// about the list: the rail is Home, Inbox, Agents, Services and the account's
// own pages, and a console sitting second among them made the rail read as
// four destinations plus an exception.
//
// It is in the header now, beside the balance — the other item there that is a
// fact about your standing rather than a page in the product. So these tests
// assert two things: an admin has the door, and it is not in the rail.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAnAdminGetsTheLinkInTheNav(t *testing.T) {
	nav := headAdmin(&auth.Account{ID: "boss", Admin: true})
	if !strings.Contains(nav, `href="/admin"`) {
		t.Fatal("an admin has no way to the dashboard from the sidebar")
	}
	if strings.Contains(nav, "display: none") {
		t.Error("the admin's own link is hidden, so it needs JavaScript to appear")
	}
	// And it is in the header, before the rail starts.
	//
	// Read by position, because that is the whole claim. #head-right is in the
	// markup above #container, so an Admin link that landed anywhere in the
	// rail — where it was, second under Home — comes after it. Asserting on the
	// id alone would pass with the link back in the list it left.
	page := renderWithLang("t", "d", "", "en", &auth.Account{ID: "boss", Admin: true})
	adm, rail, out := strings.Index(page, `id="head-admin"`), strings.Index(page, `id="nav"`),
		strings.Index(page, `id="nav-logout"`)
	if adm < 0 || rail < 0 || out < 0 {
		t.Fatalf("the shell is missing a part (admin %d, rail %d, logout %d)", adm, rail, out)
	}
	if adm > rail {
		t.Errorf("admin is inside the rail rather than in the header (admin %d, rail %d) — "+
			"the rail is the four things the product is, and an operator console is "+
			"not one of them", adm, rail)
	}
	// Nothing named nav-admin anywhere: the rail entry is gone, not duplicated.
	if strings.Contains(page, `id="nav-admin"`) {
		t.Error("the rail still draws its own Admin entry, so the door is in two places")
	}
}

// The bottom of the rail is who you are, what is yours, and the way out.
//
// It used to be a menu holding everything the account owns — Account, Profile,
// Wallet, Tokens, Admin — behind a disclosure under your own name. That was
// wrong because it *hid* destinations, and all of them moved up.
//
// Two then came back, which is not the menu returning: Account and Profile are
// the two that are about you rather than about the instance, and a flat list
// under your own name hides nothing. Wallet and Tokens stayed in the rail, and
// Admin is in neither: it has left the rail for the header.
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
	// Account came back deliberately: it is the page that is about *you*
	// rather than about the instance, and under your own name is where it
	// reads — "signed in as @someone", then the page that is @someone's, then
	// the way out.
	//
	// Profile was the other one and is gone with the page. /@somebody is the
	// conversation with them now, so your own resolves to your inbox, which is
	// already the first thing in the rail.
	for _, mine := range []string{"/account"} {
		if !strings.Contains(bottom, `href="`+mine+`"`) {
			t.Errorf("%s is not under the name, where what is yours belongs", mine)
		}
	}
	if strings.Contains(bottom, `href="/@someone"`) {
		t.Error("the rail still links a profile page that no longer exists")
	}
	// The rest stay in the rail. They are the instance's services, not your
	// account's pages, and a wallet under your name is the disclosure menu
	// growing back one item at a time.
	for _, moved := range []string{"/token", "/wallet", "/admin", "/inbox", "/agents"} {
		if strings.Contains(bottom, `href="`+moved+`"`) {
			t.Errorf("%s is in the bottom group rather than the rail", moved)
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
// the shell is rendered per viewer, so there is no cached page to defend against.
func TestAnOrdinaryAccountIsNotShownTheDoor(t *testing.T) {
	if nav := headAdmin(&auth.Account{ID: "reader"}); nav != "" {
		t.Errorf("a non-admin is offered the admin dashboard: %s", nav)
	}
	if strings.Contains(navBottom(&auth.Account{ID: "reader"}), "/admin") {
		t.Error("the bottom group still carries an admin link")
	}
}

func TestSignedOutGetsNoAdminLink(t *testing.T) {
	if headAdmin(nil) != "" || strings.Contains(navBottom(nil), "/admin") {
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
