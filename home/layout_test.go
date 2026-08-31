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
	"strings"
	"testing"

	"mu/internal/auth"
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

	// Yours, on the left. The input first because it is the one control on the
	// page, then how things are, then what arrived, then who is working — the
	// order somebody reads them in.
	//
	// The headings are matched as sectionRule writes them rather than as bare
	// words. Three of these four are also names in the nav and in the phone tab
	// bar, and the tab bar is rendered after the columns — so ">Inbox<" alone
	// finds the tab and reports the inbox block on the wrong side of the page.
	for _, want := range []string{`id="home-agent"`, sectionRule("Brief"), sectionRule("Inbox"), sectionRule("Agents")} {
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

	// The date is neither. It is a status line for the page — the day, and the
	// temperature where you are — so it spans both columns rather than being
	// the first thing in one of them.
	if strings.Contains(rail, `id="home-date"`) || strings.Contains(main, `id="home-date"`) {
		t.Error("the date is inside a column; it spans the top of both")
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
