package home

import (
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/thread"
)

// homeFor renders the signed-in Home for an account, as the browser gets it.
func homeFor(t *testing.T, accountID string, names ...string) string {
	t.Helper()

	name := accountID
	if len(names) > 0 {
		name = names[0]
	}
	auth.Create(&auth.Account{ID: accountID, Name: name}) //nolint:errcheck
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
	if !strings.Contains(body, "Welcome back, "+who) {
		t.Error("missing personal greeting")
	}
	personalAt := strings.Index(body, `<section id="home-personal"`)
	feedAt := strings.Index(body, `<section id="home-feed"`)
	if personalAt < 0 || feedAt <= personalAt {
		t.Fatal("missing view panels")
	}
	personal, feed := body[personalAt:feedAt], body[feedAt:]
	for _, want := range []string{`id="home-agent"`, `id="home-brief"`, `<a class="card-head-link" href="/inbox">Inbox</a>`} {
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

	// The heading links to the full section, like the other previews.
	if !strings.Contains(got, `<a class="card-head-link" href="/account">Account</a>`) {
		t.Error("the Account heading is not sectionRule's, so it does not match " +
			"Inbox and Agents above it")
	}
	// A card, sharing the class the other two are styled by.
	if !strings.Contains(got, `id="home-account-card" class="section-card"`) {
		t.Error("the balance is not in a card, and both blocks above it are")
	}
	if strings.Contains(got, "Go to account") {
		t.Error("redundant account footer")
	}

}

func TestGreetingTreatsDisplayNamesAsText(t *testing.T) {
	body := homeFor(t, "greetingmarkup", `<img src=x onerror=alert(1)> & friends`)
	want := `Welcome back, &lt;img src=x onerror=alert(1)&gt; &amp; friends`
	if !strings.Contains(body, `<h1 id="page-title">`+want+`</h1>`) {
		t.Error("greeting does not render the display name as text")
	}
	if strings.Contains(body, `<img src=x onerror=alert(1)>`) {
		t.Error("display name became executable markup")
	}
}

func TestUpcomingEndpointRequiresSession(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/home?section=upcoming", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("upcoming endpoint: %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestHomeBoundsItsExternalCalendarRead(t *testing.T) {
	const owner = "boundedcalendar"
	old := events.ExternalEntries
	defer func() { events.ExternalEntries = old }()
	calls := 0
	events.ExternalEntries = func(got string, from, to time.Time, limit int) []events.External {
		calls++
		if got != owner || limit != events.PreviewLimit {
			t.Fatalf("calendar request owner=%q limit=%d", got, limit)
		}
		return []events.External{{Title: "External meeting", Start: time.Now().Add(time.Hour)}}
	}
	homeFor(t, owner)
	if calls != 0 {
		t.Fatal("Home blocked on a provider request")
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/home?section=upcoming&owner=someoneelse", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != 200 || calls != 1 || !strings.Contains(rec.Body.String(), "External meeting") {
		t.Fatalf("preview %d, calls %d", rec.Code, calls)
	}
}

func TestPublicHomeDoesNotExposePersonalCards(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/home", nil))
	body := rec.Body.String()
	for _, id := range []string{"home-inbox", "home-todo", "home-agents", "home-upcoming", "mu-chat-location"} {
		if strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("public Home exposes %s", id)
		}
	}
	if !strings.Contains(body, `id="home-brief"`) || !strings.Contains(body, `id="home-feed"`) {
		t.Fatal("missing public content")
	}
}
