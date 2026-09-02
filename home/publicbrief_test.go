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
)

func TestTheLandingCarriesTodaysBrief(t *testing.T) {
	const line = "Oil slipped below $70 as Gulf tensions eased."

	got := briefBlock(line)
	if !strings.Contains(got, line) {
		t.Fatalf("the line is not on the page: %q", got)
	}
	if !strings.Contains(got, "data-brief") {
		t.Error("the brief is not marked data-brief, so asking a question will\n" +
			"leave it on the page and push it under the answer")
	}
	// Dated, because a brief with no date could be from any morning and the
	// claim is that this one is today's.
	if !strings.Contains(got, "lbrief-day") {
		t.Error("the brief is not dated")
	}
}

// Nothing at all when there is no line — a new instance, a model that has not
// run, or the first hours of a day. A heading over nothing is worse than
// nothing, and this page is built to fit on one screen.
func TestNoLineIsNoBlock(t *testing.T) {
	for _, line := range []string{"", "   ", "\n\t "} {
		if got := briefBlock(line); got != "" {
			t.Errorf("briefBlock(%q) rendered %q", line, got)
		}
	}
}

// It is the world's day, so it is escaped like anything else off a model.
func TestTheBriefIsEscaped(t *testing.T) {
	got := briefBlock(`markets <script>alert(1)</script> fell`)
	if strings.Contains(got, "<script>") {
		t.Errorf("a line went onto the landing page as markup: %q", got)
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
