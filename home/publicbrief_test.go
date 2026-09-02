package home

// The public brief, and where it goes when somebody asks.
//
// Two things are being pinned. The first is that a stranger gets today's brief
// on the landing page without an account — which is the whole argument the page
// makes, demonstrated rather than described. The second is that the brief and
// an answer are never on the screen together: they are the same shape, a
// paragraph of prose, and stacked they read as one thing with a seam in it.
// Worse, every turn of a conversation pushes the brief further down a page it
// was the point of.
//
// You are told, or you ask.

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/service"
)

func TestTheLandingCarriesToday(t *testing.T) {
	got := today("")
	if got == "" {
		t.Skip("nothing fetched in this build, so there is no day to show")
	}
	if !strings.Contains(got, "data-brief") {
		t.Error("today is not marked data-brief, so asking a question will\n" +
			"leave it on the page and push it under the answer")
	}
	// Dated once, at the top. Three timestamps on three rows is a page about
	// its own freshness.
	if !strings.Contains(got, "lday") {
		t.Error("the block is not dated")
	}
	if n := strings.Count(got, "lday"); n != 1 {
		t.Errorf("the date appears %d times, want once", n)
	}
}

// Nothing at all when there is no line — a new instance, a model that has not
// run, or the first hours of a day. A heading over nothing is worse than
// nothing, and this page is built to fit on one screen.
func TestNothingToSayIsNoBlock(t *testing.T) {
	if got := briefRow(""); got != "" && strings.TrimSpace(got) == "" {
		t.Errorf("an empty brief still rendered a row: %q", got)
	}
	// With no clauses at all there is no row, and with no rows at all there is
	// no block — no date floating over nothing.
	if len(briefParts("")) == 0 && briefRow("") != "" {
		t.Error("a brief with no clauses rendered a row anyway")
	}
}

// The rows that carry third-party text escape it. The brief is the exception
// and says so: its clauses are built with their own links by home/brief.go.
func TestTheRowsOffThirdPartiesAreEscaped(t *testing.T) {
	if got := marketsRow(); strings.Contains(got, "<script") {
		t.Errorf("the markets line went onto the page as markup: %q", got)
	}
}

// The headlines are not on the front door, because the brief is written from
// them.
//
// agent/brief reads the headlines and writes a sentence about the day — see its
// sources — so printing both put the working above the answer, and the answer
// is the better of the two because somebody judged it. A front page that prints
// its own source material is a feed with a summary at the top.
func TestTheFrontDoorDoesNotPrintTheBriefsSources(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "news.GetFeed()") {
		t.Error("the front door reads the news feed again. The brief is written\n" +
			"from it, so this is the same day twice — the summary and the\n" +
			"material it summarises, one above the other.")
	}
}

// The mechanism, in the one place that owns it.
//
// Both briefs — the landing's and Home's — are plain markup with an attribute
// on them, and every bit of the behaviour is in the chat component. So this
// reads the component: it has to hide on a question, put it back on a fresh
// session, and stay hidden on a reload that still has a conversation in it.
// Three lines, and losing any one of them is a bug nobody would see in a diff.
func TestAskingTakesTheBriefOffThePage(t *testing.T) {
	js := app.ChatComponent(app.ChatConfig{Ask: true, HideSuggestions: true})

	for _, want := range []struct{ what, snippet string }{
		{"a way to find the briefs", "[data-brief]"},
		{"hiding when a question is asked", "hideBrief();\n  sugDiv.innerHTML='';"},
		{"putting it back on a fresh session", "showBrief();"},
		{"staying hidden when a conversation is restored", "if(conv.innerHTML.trim())hideBrief();"},
	} {
		if !strings.Contains(js, want.snippet) {
			t.Errorf("the chat component has no %s (looked for %q)", want.what, want.snippet)
		}
	}
}

// And Home's brief takes part, so the two pages behave the same way.
//
// Home puts the answer directly above it — the conversation is inside
// #home-agent and the brief is the block after it — so without the attribute
// every turn walks the brief down the page.
func TestHomesBriefIsMarkedToo(t *testing.T) {
	src, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `id="home-brief" data-brief`) {
		t.Error("Home's brief is not marked data-brief, so it stays on the page\n" +
			"under the answer while the landing page's steps aside")
	}
}

// Two pages, for two audiences.
//
// This went round twice. First the landing was one page in two states, on the
// argument that signing in should not move you somewhere else and a question
// half-typed in the box should survive it. Both true, and both smaller than
// what they cost: a landing page is written for somebody deciding whether they
// want this, and serving it to somebody who already has an account makes the
// app's front door an argument aimed at a stranger. Then the landing was folded
// into the app shell instead, which put a hamburger over a wordmark and, on a
// phone, a tab bar under it.
//
// The app has Home in its tab bar. You press Home and you know where you are.
// So: signed out, a landing; signed in, /home — which has the rail, the tabs,
// the inbox and the same box to type in.
func TestSigningInLeavesTheLanding(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `http.Redirect(w, r, "/home", http.StatusSeeOther)`) {
		t.Error("the landing page is served to people who are signed in, so the\n" +
			"app's front door is a pitch aimed at somebody who already bought")
	}
	// And it is not a dashboard either. The moment this grows a column of your
	// inbox and your agents it is /home with worse navigation.
	body := indexBody()
	for _, dashboard := range []string{"home-cards", "home-rail", "home-main"} {
		if strings.Contains(body, dashboard) {
			t.Errorf("%q is on the landing — that is the dashboard, and it lives at /home", dashboard)
		}
	}
	// The box is still the thing on it.
	for _, want := range []string{"mu-chat-input", "mu-chat-form"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q is not on the landing", want)
		}
	}
}

// The front door has no rail, and does not grow one.
//
// It was folded into the app shell for an afternoon, on a misreading: the
// complaint was that logged-out and logged-in did not match "on any pages that
// use the header and sidebar", and this is not one of those pages. What came
// back was a hamburger over a landing page and, on a phone, a tab bar under it
// — the app wearing a landing page's clothes.
//
// The corner is still the thing that says which state you are in. Here it is
// this page's own, because this page has no rail to put a way out in; on every
// app page it is the shell's. See app.headCorner.
func TestTheFrontDoorIsItsOwnPage(t *testing.T) {
	rec := httptest.NewRecorder()
	Index(rec, httptest.NewRequest("GET", "/", nil))
	page := rec.Body.String()

	for _, chrome := range []struct{ what, snippet string }{
		{"the app's rail", `id="nav-container"`},
		{"the hamburger", `id="menu-toggle"`},
		{"the phone tab bar", `id="tabs"`},
		{"a page title", `id="page-title"`},
	} {
		if strings.Contains(page, chrome.snippet) {
			t.Errorf("the front door carries %s (%s) — that is the app shell, and\n"+
				"a landing page with a hamburger on it is the app in a costume",
				chrome.what, chrome.snippet)
		}
	}
	// And it still says which state you are in.
	if !strings.Contains(page, `href="/login"`) {
		t.Error("signed out, the front door offers no way in")
	}
}

// The signed-in brief is the signed-out one plus the clauses that need an
// account. One function decides what a brief says; nothing else does.
func TestSigningInAddsToTheBriefRatherThanReplacingIt(t *testing.T) {
	world := briefParts("")
	for _, p := range world {
		if strings.TrimSpace(p) == "" {
			t.Error("an empty clause is in the brief")
		}
	}
	// Signed out, only the world's clause can be there — everything else is
	// read from an account.
	if len(world) > 1 {
		t.Errorf("a signed-out brief has %d clauses, want at most the world's one: %v", len(world), world)
	}
}

// The doors and the guest's tools are one list.
//
// The row under the box is a promise about what this instance can answer, and
// the tools behind a guest's question are how it keeps the promise. Two lists
// would be one that had drifted, and the drift is invisible from either side:
// the page offers a door the agent cannot walk through, and nothing looks wrong
// on either.
//
// Read from the source, because the registry is empty in a unit test and the
// thing worth pinning is that there is one list rather than what is in it.
func TestTheDoorsAreTheToolsAGuestCanReach(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "range service.Guest()") {
		t.Error("the row of doors is not drawn from service.Guest, so the page can\n" +
			"offer a door the agent has no tool for")
	}

	agentSrc, err := os.ReadFile("../agent/native.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agentSrc), "return service.Guest()") {
		t.Error("a guest's tools are not service.Guest's, so every stranger's\n" +
			"question carries every service that is not account-scoped —\n" +
			"two dozen of them, in the prompt, paid for per question")
	}
}

// And nothing account-scoped is ever in it. Somebody's mail, contacts, files
// and wallet are the whole reason this filter exists.
func TestNoGuestToolReachesAnAccount(t *testing.T) {
	for _, name := range service.Guest() {
		if service.AccountScoped(name) {
			t.Errorf("%s is account-scoped and is offered to signed-out callers", name)
		}
	}
}

// The doors row is the front door's, and only the front door's.
//
// It was on both, on the argument that the box is the one control the two pages
// share. That is consistency of the component rather than of the page: the row
// is a way to reach the services, the front door has no other one, and Home has
// two — Services in the rail and a grid of those same services below the fold.
// On Home it was a third row of furniture under one input, above the agent
// picker and the read-aloud toggle, duplicating a rail six inches to the left.
func TestOnlyTheFrontDoorCarriesTheDoors(t *testing.T) {
	src, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "Doors:") {
		t.Error("Home draws the row of doors again — it already reaches the\n" +
			"services twice, from the rail and from the grid below")
	}

	index, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "Doors: directDoors(),") {
		t.Error("the front door has lost the row, and it is the page with no\n" +
			"other way to reach a service")
	}
}
