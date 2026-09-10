package home

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

// Home is the personal workspace; Feed holds the existing service cards.
func TestHomeAndFeedKeepTheirOwnContent(t *testing.T) {
	const who = "homefeedreader"
	th := thread.Open(who, "mail", "sender@example.com")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, From: "sender@example.com", Text: "An inbox message"})
	body := homeFor(t, who)
	personalAt := strings.Index(body, `<section id="home-personal"`)
	feedAt := strings.Index(body, `<section id="home-feed"`)
	if personalAt < 0 || feedAt <= personalAt {
		t.Fatal("missing view panels")
	}
	personal, feed := body[personalAt:feedAt], body[feedAt:]
	for _, want := range []string{`id="home-agent"`, `id="home-brief"`, sectionRule("Inbox")} {
		if !strings.Contains(personal, want) || strings.Contains(feed, want) {
			t.Errorf("%s is not exclusive to Home", want)
		}
	}
	if strings.Contains(personal, `class="home-main`) || !strings.Contains(feed, `class="home-main full"`) {
		t.Error("service cards are not exclusive to Feed")
	}
	if !strings.Contains(body, `id="home-view-personal" href="/home" role="tab" aria-controls="home-personal" aria-selected="true"`) {
		t.Error("Home is not the default")
	}
	if !strings.Contains(body, `id="home-feed" role="tabpanel" aria-labelledby="home-view-feed" hidden`) {
		t.Error("Feed is initially visible")
	}
}

func TestFeedLinksAndDisplayMode(t *testing.T) {
	for _, path := range []string{"/home?view=feed", "/home?mode=display"} {
		rec := httptest.NewRecorder()
		Handler(rec, httptest.NewRequest("GET", path, nil))
		body := rec.Body.String()
		if !strings.Contains(body, `id="home-personal" role="tabpanel" aria-labelledby="home-view-personal" hidden`) {
			t.Errorf("%s did not hide Home", path)
		}
		if strings.Contains(body, `id="home-feed" role="tabpanel" aria-labelledby="home-view-feed" hidden`) {
			t.Errorf("%s hid Feed", path)
		}
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
	if !strings.Contains(got, `href="/account"`) {
		t.Error("nothing on Home leads to /wallet")
	}
	if !strings.Contains(got, "credits") {
		t.Error("the number has no unit on it")
	}
	if strings.Contains(got, "1 credit =") {
		t.Error("Home repeats the account conversion note")
	}
	for _, want := range []string{"/account/topup", "/account/transfer"} {
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
func TestTheAccountBlockLooksLikeTheOtherRailBlocks(t *testing.T) {
	was := quota.Enabled
	quota.Enabled = func() bool { return true }
	t.Cleanup(func() { quota.Enabled = was })

	got := walletHTML("walletshape")
	if !strings.Contains(got, ">Balance</h4>") || strings.Contains(got, `href="/wallet"`) {
		t.Fatal("account card has wrong label or destination")
	}

	// The heading is the same plain one the others use, not a link.
	if !strings.Contains(got, sectionRule("Account")) {
		t.Error("the Account heading is not sectionRule's, so it does not match " +
			"Inbox and Agents above it")
	}
	// A card, sharing the class the other two are styled by.
	if !strings.Contains(got, `class="wallet-peek"`) {
		t.Error("the balance is not in a card, and both blocks above it are")
	}
	// And the way to the page, where the others put it.
	if !strings.Contains(got, `href="/account" class="link"`) {
		t.Error("no `Go to account` link — every other rail block ends with one, " +
			"and it is the only route to /wallet from Home")
	}
}
