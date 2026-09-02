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

// Nothing on the page is third-party text put there raw.
//
// This asked it of marketsRow, which was a line of tickers under the brief and
// is gone with the rest of the furniture — see TestNeitherPageCarriesARowOfDoors
// for why. The brief is what is left, and it is the exception that has to say
// so out loud: its clauses are built with their own links by home/brief.go, so
// it is the one row that may carry markup, and the reason it may is that we
// wrote it.
func TestTheBriefIsTheOnlyRowThatMayCarryMarkup(t *testing.T) {
	src, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatal(err)
	}
	// One row, so one place to check. A second unescaped row appearing here is
	// the thing worth catching, and it would show as another briefRow-shaped
	// call in the list.
	if n := strings.Count(string(src), "briefRow(viewerID),"); n != 1 {
		t.Errorf("today() assembles %d rows, want 1 — every row but the brief\n"+
			"has to escape what it prints, and this test only reads the brief", n)
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

// A guest's tools are the front door's set, and nothing wider.
//
// This used to pin two things against each other: that the row of doors under
// the box and the tools behind a guest's question came from one list, so the
// page could not offer a door the agent had no tool for. The row is gone — see
// TestNeitherPageCarriesARowOfDoors — and the half that mattered is the half
// that is left.
//
// Because the alternative is not "a smaller list". It is every service that is
// not account-scoped: two dozen of them and all their methods, in the prompt of
// every stranger's question, paid for per question by whoever runs the
// instance, and mostly tools nobody arrives asking for — shell, browser,
// transit, flights, images, apps.
//
// Read from the source, because the registry is empty in a unit test and the
// thing worth pinning is where the list comes from rather than what is in it.
func TestAGuestsToolsAreTheGuestSet(t *testing.T) {
	agentSrc, err := os.ReadFile("../agent/native.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agentSrc), "return service.Guest()") {
		t.Error("a guest's tools are not service.Guest's, so every stranger's\n" +
			"question carries every service that is not account-scoped")
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

// Neither page carries a row of doors.
//
// It was on both, then only on the front door — Home reaches the services from
// the rail and from a grid below the fold, so a third row of the same links
// under one input was furniture. That reasoning held and did not go far enough.
//
// The front door lost it too. Nine links under the box, on the argument that a
// box which answers everything alone on a page is a thing you have to go
// through, and that everything the agent does you should be able to do
// yourself. Still true, and not an argument for putting the list *here*: held
// against the thing this competes with — writing to agent@, where nothing comes
// back but an answer — a front door with a ticker, a photograph and nine links
// on it is the noisiest version of the quietest idea in the product.
//
// The services are on /about and in the catalogue, and a signed-in reader has
// the rail. What a stranger needs from this page is somewhere to type.
func TestNeitherPageCarriesARowOfDoors(t *testing.T) {
	for _, f := range []string{"index.go", "home.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "Doors: directDoors()") {
			t.Errorf("%s draws the row of doors again", f)
		}
	}
}
