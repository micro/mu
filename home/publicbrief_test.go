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
	if got := headlinesRow(); strings.Contains(got, "<script") {
		t.Errorf("a headline went onto the page as markup: %q", got)
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

// One front door, in two states.
//
// Signing in used to move you to /home, so the page a person chose to visit was
// replaced by a different one the moment they had an account — and a question
// half-typed in the box did not survive the move. This is the same page either
// way; what changes is the corner and the brief.
func TestTheFrontDoorIsTheSamePageSignedInOrOut(t *testing.T) {
	out := indexBody("")
	in := indexBody("somebody")

	for _, want := range []string{"mu-chat-input", "mu-chat-form"} {
		if !strings.Contains(out, want) || !strings.Contains(in, want) {
			t.Errorf("%q is not on both states of the front door", want)
		}
	}
	// And no rail. The moment this grows a column of your inbox and your
	// agents it is /home with worse navigation, and there is no reason for two
	// of those.
	for _, dashboard := range []string{"home-cards", "home-rail", "home-main"} {
		if strings.Contains(in, dashboard) {
			t.Errorf("%q is on the front door — that is the dashboard, and it lives at /home", dashboard)
		}
	}
}

// The corner is the only thing that says whether anybody is signed in.
func TestTheCornerIsTheWayInOrTheWayDeeper(t *testing.T) {
	out := topRight("")
	if !strings.Contains(out, `href="/login"`) {
		t.Errorf("signed out, the corner does not offer a way in: %q", out)
	}
	if !strings.Contains(out, `href="/signup"`) {
		t.Errorf("signed out, the corner does not offer a way to join: %q", out)
	}
	if strings.Contains(out, `href="/home"`) {
		t.Errorf("signed out, the corner offers Home: %q", out)
	}

	in := topRight("somebody")
	if !strings.Contains(in, `href="/home"`) {
		t.Errorf("signed in, the corner does not reach the dashboard: %q", in)
	}
	if strings.Contains(in, `href="/login"`) {
		t.Errorf("signed in, the corner still says sign in: %q", in)
	}
	// Two links either side of the line, and no third. Install app stood here
	// and did not earn the slot: it appeared only on some browsers and said
	// nothing about what state you are in or what to do about it, which is the
	// corner's whole job. Browsers offer installing in their own menus.
	for _, s := range []string{out, in} {
		if strings.Contains(s, "install-app") {
			t.Errorf("Install is back in the corner: %q", s)
		}
		if n := strings.Count(s, "<a "); n != 2 {
			t.Errorf("the corner has %d links, want 2: %q", n, s)
		}
	}

	// And the way out is here, because this page has no rail to put it in.
	if !strings.Contains(in, `href="/logout"`) {
		t.Errorf("signed in, there is no way to sign out: %q", in)
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

// And Home shows the same row, from the same list, in the same place. It is the
// one control both pages share, and it read as a different product on each
// while only one of them had the doors.
func TestHomeHasTheDoorsToo(t *testing.T) {
	src, err := os.ReadFile("home.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "Doors:            directDoors(),") &&
		!strings.Contains(string(src), "Doors: directDoors(),") {
		t.Error("Home's box has no doors under it, so the two surfaces that share\n" +
			"one control offer different things around it")
	}
}
