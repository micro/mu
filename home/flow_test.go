package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The landing is one screen about one thing.
//
// It has been three different pages. Three buttons and three cards, all about
// the tools and nothing you could do with them. Then a working chat box, on the
// ollama argument — nobody wants a model, they want to use one. Then that plus
// three feature cards plus a Connect via MCP section with a four-step list.
//
// The last one is what this replaced, and the problem was not any single part:
// each was true and each argued for a different thing. A visitor here is
// deciding whether they want an agent at all, and that is one question. How to
// point Cursor at the endpoint is a real question asked by somebody who has
// already decided, and it belongs on /tools, which is the page about tools.
//
// So: a headline, a sentence, the address, and two ways on. It fits on a screen
// and there is nothing below it.
func TestTheLandingIsOneScreenAboutOneThing(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	// What it is, and the one fact that makes it different.
	for _, want := range []string{"A personal agent", "@", `class="laddr"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing is missing %q", want)
		}
	}
	// And two ways on, no more.
	for _, want := range []string{`href="/signup"`, `href="/tools"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing has no path to %s", want)
		}
	}

	// What it must not carry any more. Each of these was a section arguing for
	// something else on a page that should argue for one thing.
	for what, marker := range map[string]string{
		"a chat box":           "mu-chat-form",
		"feature cards":        `class="lcards"`,
		"the MCP setup steps":  "Connect via MCP",
		"a payment rail pitch": "x402",
	} {
		if strings.Contains(body, marker) {
			t.Errorf("the landing still carries %s — that is a second thing to "+
				"decide about, and it belongs on the page about it", what)
		}
	}
}

// The line is said once.
//
// The page carried "An Inbox for Agents" in the chrome and "A personal agent."
// as its headline — the line this positioning replaced, surviving in the one
// place nothing rendered next to the other. Retiring the old line by writing
// the new one into the same slot fixed the contradiction and left the
// duplication: the same sentence twice, once at 18px and once at 38px, on a
// page whose whole argument is that it says one thing.
//
// So the rule is a count, not an absence — an absent old line is what the
// previous version of this test checked, and it passed the whole time the page
// said it twice. The <head> is excluded on purpose: the <title> and the og
// description are allowed to repeat the pitch, because nobody reads them
// beside it.
func TestThereIsOneTagline(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	page := rec.Body.String()

	if strings.Contains(page, "An Inbox for Agents") {
		t.Error("the landing still carries the old tagline above the new headline, " +
			"so a visitor reads two different pitches stacked")
	}

	body := page
	if i := strings.Index(page, "<body>"); i >= 0 {
		body = page[i:]
	}
	if n := strings.Count(body, "A personal agent"); n != 1 {
		t.Errorf("the landing says %q %d times on one screen, want once — it is the "+
			"headline, and a second copy in the chrome above it is the same "+
			"sentence in a smaller font", "A personal agent", n)
	}
}
