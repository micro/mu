package home

// Which side of Home a block lands on.
//
// Home is a single stack of blocks written one after another, and above 1024px
// mu.css turns that stack into two columns by wrapping it in .home-rail and
// .home-main. Nothing in the CSS names a block: the split is entirely which
// side of one WriteString a block was written on, so a block moved a few lines
// in Handler changes column silently and nothing anywhere says it did.
//
// That is the failure this pins. The rule is not an arrangement somebody liked
// — it is whose the content is. The rail is this account: what you can ask, how
// things are, what arrived, who is working, all of it short by nature. The main
// column is the instance reading the world, and is a card grid that wants
// width. Giving both the same width was the layout Home had at every size, and
// it wasted whichever one it was not sized for.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/thread"
)

// homeFor renders the signed-in Home for an account, as the browser gets it.
func homeFor(t *testing.T, accountID string) string {
	t.Helper()

	auth.Create(&auth.Account{ID: accountID, Name: accountID}) //nolint:errcheck
	sess, err := auth.CreateSession(accountID)
	if err != nil {
		t.Fatalf("no session: %v", err)
	}

	r := httptest.NewRequest("GET", "/home", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	rec := httptest.NewRecorder()
	Handler(rec, r)
	return rec.Body.String()
}

// The page has two sides, and the blocks are on the right ones.
//
// Read by position rather than by class, because position is the whole
// mechanism: .home-rail and .home-main are two plain divs with nothing in them
// but the order Handler wrote. A test that only looked for the class names
// would pass with every block on one side.
func TestHomeSplitsWhatIsYoursFromWhatIsTheWorlds(t *testing.T) {
	const who = "layoutsplit"

	// Something to report, or three of the four blocks draw nothing and this
	// checks only that the wrappers exist. One arrived conversation gives the
	// inbox a row and the brief a clause about it.
	th := thread.Open(who, "mail", "<layout@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Join(who, th.ID, thread.Party{Kind: thread.RolePerson,
		Key: "henrik@example.com", Name: "Henrik"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com"})

	body := homeFor(t, who)
	rail, main, ok := splitHome(body)
	if !ok {
		t.Fatal("Home has no .home-rail / .home-main split, so it is one column " +
			"at every width again")
	}

	// Yours, on the left: what arrived, then who is working, then what it is
	// being paid for — the order somebody reads them in.
	//
	// Brief is not in this list any more. It spans both columns with the box:
	// the rail is lists and the brief is a sentence about all of them. See
	// TestTheBriefIsWithTheBoxAndNotInTheRail, which pins that from the
	// other side.
	//
	// The headings are matched as sectionRule writes them rather than as bare
	// words. Both are also names in the nav and in the phone tab bar, and the
	// tab bar is rendered after the columns — so ">Inbox<" alone finds the tab
	// and reports the inbox block on the wrong side of the page.
	for _, want := range []string{sectionRule("Inbox"), sectionRule("Agents")} {
		if !strings.Contains(rail, want) {
			t.Errorf("%s is not in the rail, so it takes the width the cards need", want)
		}
		if strings.Contains(main, want) {
			t.Errorf("%s is in the main column as well as, or instead of, the rail", want)
		}
	}

	// The world's, on the right. This is the block that was below the fold on
	// every screen, which is the whole reason for the split.
	if !strings.Contains(main, sectionRule("Services")) {
		t.Error("the services are not in the main column, so they are still a " +
			"narrow strip under everything else")
	}
	if strings.Contains(rail, sectionRule("Services")) {
		t.Error("the services are in the rail, which is the one place too narrow " +
			"for a grid of cards")
	}

	// The date and the box are in neither column. Both are about the whole page
	// — one says what day it is and how warm, the other is where you type — so
	// they run across the top of both.
	//
	// The box is here rather than in the list above because it spent a commit
	// in the rail and had to come out. Everything in the rail is a list and a
	// control is not: a text field indented to a third of the page, with a grid
	// of cards starting beside it, reads as a widget in a sidebar rather than
	// as the thing the page is for.
	for _, id := range []string{`id="home-date"`, `id="home-agent"`} {
		if strings.Contains(rail, id) || strings.Contains(main, id) {
			t.Errorf("%s is inside a column; it spans the top of both", id)
		}
	}
}

// splitHome cuts the rendered page into the two columns.
//
// Returns false rather than empty strings when the wrappers are gone, so a
// caller cannot mistake "Home is one column again" for "both columns are
// empty" — which is the shape this test exists to catch.
func splitHome(body string) (rail, main string, ok bool) {
	const (
		open = `<div class="home-rail">`
		mid  = `</div><div class="home-main">`
	)
	i := strings.Index(body, open)
	if i < 0 {
		return "", "", false
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, mid)
	if j < 0 {
		return "", "", false
	}
	return rest[:j], rest[j+len(mid):], true
}

// With nothing in the rail there is no rail, and the world takes the page.
//
// The rail's three blocks all belong to an account, so signed out all three are
// empty and the wrapper held nothing. It was still a grid track: 320px of blank
// on the left of the first screen a visitor ever sees, with the services
// starting a third of the way across from a column that is not there. Nothing
// on the page distinguished that from the layout being broken, which is what it
// looked like.
//
// A new account is the same page — briefHTML, inbox.Preview and agent.Preview
// each decide their own silence, and on a fresh account all three are silent —
// which is why the condition is what is in the rail and not who is reading.
func TestWithNothingInTheRailThereIsNoRail(t *testing.T) {
	r := httptest.NewRequest("GET", "/home", nil)
	rec := httptest.NewRecorder()
	Handler(rec, r)
	body := rec.Body.String()

	if strings.Contains(body, `<div class="home-rail">`) {
		t.Error("a signed-out Home still opens a rail; empty, it is a 320px grid " +
			"track of nothing beside the services")
	}
	// And the column that is left says it is the whole width, or the grid puts
	// it in the second track regardless — see .home-main.full in mu.css.
	if !strings.Contains(body, `<div class="home-main full">`) {
		t.Error("the main column is not marked full, so it stays in the second " +
			"grid track with an empty first one beside it")
	}

	// The services really are on the page — otherwise this passes on a Home
	// that rendered nothing at all.
	if !strings.Contains(body, "Services") {
		t.Error("no services on a signed-out Home, so there is nothing for the " +
			"full-width column to hold")
	}
}

// And a rail with something in it is still a rail.
func TestARailWithContentIsStillWritten(t *testing.T) {
	const who = "railcontent"

	th := thread.Open(who, "mail", "<rail@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Join(who, th.ID, thread.Party{Kind: thread.RolePerson,
		Key: "henrik@example.com", Name: "Henrik"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com"})

	body := homeFor(t, who)
	if _, _, ok := splitHome(body); !ok {
		t.Fatal("an account with mail waiting has no rail")
	}
	if strings.Contains(body, `<div class="home-main full">`) {
		t.Error("the main column claims both tracks while a rail is beside it")
	}
}

// The brief is with the box, not in the rail.
//
// It was the rail's first block. The rail is three lists — what arrived, who is
// working, what is left — and the brief is not a list, it is a sentence about
// all of them; indented to a third of the page under a full-width input it read
// as a caption on the box. It spans both columns now, with the box: you ask, or
// you are told, and everything below is a place to look.
func TestTheBriefIsWithTheBoxAndNotInTheRail(t *testing.T) {
	const who = "briefplace"

	th := thread.Open(who, "mail", "<brief@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Join(who, th.ID, thread.Party{Kind: thread.RolePerson,
		Key: "henrik@example.com", Name: "Henrik"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com"})

	body := homeFor(t, who)
	rail, _, ok := splitHome(body)
	if !ok {
		t.Fatal("no rail at all")
	}
	if strings.Contains(rail, `id="home-brief"`) {
		t.Error("the brief is back inside the rail, where it is a paragraph in a " +
			"column of lists")
	}
	// And it is on the page, above the rail — otherwise this passes because the
	// brief stopped rendering.
	at := strings.Index(body, `id="home-brief"`)
	if at < 0 {
		t.Fatal("no brief on the page at all")
	}
	if railAt := strings.Index(body, `class="home-rail"`); railAt >= 0 && at > railAt {
		t.Error("the brief is below the rail rather than above both columns")
	}
}

// The balance is in the rail, and only where the header's chip is not.
func TestTheBalanceIsInTheRailOnHome(t *testing.T) {
	const who = "walletrail"

	was := quota.Enabled
	quota.Enabled = func() bool { return true }
	t.Cleanup(func() { quota.Enabled = was })

	got := walletHTML(who)
	if got == "" {
		t.Fatal("no balance block on an instance that charges")
	}
	if !strings.Contains(got, `href="/wallet"`) {
		t.Error("nothing on Home leads to /wallet")
	}
	if !strings.Contains(got, "credits") {
		t.Error("the number has no unit on it")
	}
	// The real card's contents, not a hand-rolled number. Those two links are
	// what somebody looking at a balance has come to do, and a figure with no
	// rate beside it does not say what a credit is.
	for _, want := range []string{"/wallet/topup", "/wallet/transfer", "1 credit = 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("the balance block is missing %s — it should be account.BalanceBody, "+
				"not a number written again here", want)
		}
	}

	// And nothing at all where nobody is charged, which is the same condition
	// the header's chip uses — the two must never disagree about whether money
	// exists on this instance.
	quota.Enabled = func() bool { return false }
	if walletHTML(who) != "" {
		t.Error("a balance is drawn on an instance that does not charge")
	}
}

// The header keeps its balance on Home, so the corner does not change shape.
//
// This hid it for one commit, on the rule that the same number twice on one
// screen reads as a mistake. What that produced was worse: the header gained
// and lost an item as you moved between pages — Admin alone on Home, Admin and
// a balance everywhere else — so the one piece of chrome meant to be identical
// everywhere was the piece that moved.
func TestTheHeaderBalanceIsNotHiddenOnHome(t *testing.T) {
	b, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "page-home #head-wallet") {
		t.Error("Home hides the header's balance again, so the top right has one " +
			"item on Home and two everywhere else")
	}
}

// And the wallet block is shaped like the blocks it sits under.
//
// It shipped as a bare heading with a link in it and an unbordered div, beneath
// two bordered cards with plain headings — the only thing in the rail that did
// not look like the rail.
func TestTheWalletBlockLooksLikeTheOtherRailBlocks(t *testing.T) {
	was := quota.Enabled
	quota.Enabled = func() bool { return true }
	t.Cleanup(func() { quota.Enabled = was })

	got := walletHTML("walletshape")

	// The heading is the same plain one the others use, not a link.
	if !strings.Contains(got, sectionRule("Wallet")) {
		t.Error("the Wallet heading is not sectionRule's, so it does not match " +
			"Inbox and Agents above it")
	}
	// A card, sharing the class the other two are styled by.
	if !strings.Contains(got, `class="wallet-peek"`) {
		t.Error("the balance is not in a card, and both blocks above it are")
	}
	// And the way to the page, where the others put it.
	if !strings.Contains(got, `href="/wallet" class="link"`) {
		t.Error("no `Go to wallet` link — every other rail block ends with one, " +
			"and it is the only route to /wallet from Home")
	}
}
