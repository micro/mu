package test

// A template and its arguments have to agree.
//
// fmt.Sprintf does not fail when they do not. One placeholder more than
// arguments prints "%!s(MISSING)" where the last one was and shifts every
// argument before it up a slot — so the page title lands in the sidebar, the
// nav lands where the balance goes, and the body ends with MISSING. Nothing
// errors, nothing logs, and the page still renders.
//
// That happened here: a slot for the sidebar's nested mailbox list outlived the
// list, and it took a person looking at the screen to notice, because a test
// asserting "the body is in the output" passes when the body is in the wrong
// slot.
//
// So this renders the real shells and looks for the tell.

import (
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/auth"
)

// badVerb is what Sprintf leaves behind when the two disagree, in either
// direction: too few arguments, or too many.
var badVerb = []string{"%!s(MISSING)", "%!(EXTRA", "%!d(MISSING)", "%!(NOVERB)"}

func TestTheShellFillsEverySlot(t *testing.T) {
	for what, out := range map[string]string{
		"signed out": app.RenderHTML("PAGE-TITLE", "a description", "<p>THE-BODY</p>", nil),
		"signed in": app.RenderHTML("PAGE-TITLE", "a description", "<p>THE-BODY</p>",
			&auth.Account{ID: "someone", Name: "Someone"}),
		"index": app.RenderIndex(app.Index{
			Title: "PAGE-TITLE", Description: "a description",
			Brand: "Mu", Body: "<p>THE-BODY</p>",
		}),
	} {
		for _, bad := range badVerb {
			if strings.Contains(out, bad) {
				t.Errorf("%s: the shell rendered %s — its placeholders and its "+
					"arguments disagree", what, bad)
			}
		}
		if !strings.Contains(out, "THE-BODY") {
			t.Errorf("%s: the body is not on the page", what)
		}
	}
}

// The title belongs in the heading and the tab, not in the furniture.
//
// This is the shift the missing argument caused, and it is worth asserting on
// its own: "no MISSING anywhere" would pass if the count were right and the
// order wrong.
func TestThePageTitleIsNotInTheSidebar(t *testing.T) {
	out := app.RenderHTML("PAGE-TITLE", "a description", "<p>body</p>",
		&auth.Account{ID: "someone", Name: "Someone"})

	nav := section(out, `<div id="nav"`, `<div id="content"`)
	if nav == "" {
		t.Fatal("no sidebar in the rendered shell")
	}
	if strings.Contains(nav, "PAGE-TITLE") {
		t.Errorf("the page title is rendered inside the sidebar:\n%s", nav)
	}
	// And it is where it belongs.
	if !strings.Contains(out, `id="page-title">PAGE-TITLE`) {
		t.Error("the page title is not in the heading")
	}
	if !strings.Contains(out, "<title>PAGE-TITLE") {
		t.Error("the page title is not in the tab")
	}
}

// The four hubs are in reach of a thumb.
//
// This tested an envelope in the header instead, which pointed at /mail for as
// long as it existed and was corrected to /inbox — and the whole time it was
// display:none in the stylesheet with nothing anywhere turning it on. The test
// checked the href and never whether anybody could see it, so it went on
// passing over an invisible element for as long as that element existed.
//
// So it checks the destinations and the count now. On a phone the rail is
// behind a hamburger and this bar is how anything is reached; four is what a
// tab bar holds before the labels stop being readable, and it is the reason
// Tokens and Wallet are not in it.
func TestThePhoneCarriesTheFourHubs(t *testing.T) {
	out := app.RenderHTML("A page", "a description", "<p>body</p>",
		&auth.Account{ID: "someone", Name: "Someone"})

	tabs := section(out, `<nav id="tabs"`, `</nav>`)
	if tabs == "" {
		t.Fatal("no tab bar in the rendered shell")
	}
	for _, href := range []string{"/home", "/inbox", "/agents", "/services"} {
		if !strings.Contains(tabs, `href="`+href+`"`) {
			t.Errorf("the tab bar does not reach %s:\n%s", href, tabs)
		}
	}
	if n := strings.Count(tabs, "<a "); n != 4 {
		t.Errorf("the tab bar holds %d tabs, want 4 — five stops being readable "+
			"and the rail is still there for the rest:\n%s", n, tabs)
	}
	// Not the mail store. That was the bug in the thing this replaced.
	if strings.Contains(tabs, `href="/mail"`) {
		t.Errorf("a tab opens the mail store rather than the inbox:\n%s", tabs)
	}

	// Signed out the shell is a landing page whose job is one button, and a
	// fixed bar across the bottom of it competes with that button.
	if out := app.RenderHTML("A page", "a description", "<p>body</p>", nil); strings.Contains(out, `id="tabs"`) {
		t.Error("a signed-out visitor gets the tab bar")
	}
}

// section returns what lies between two markers, or "" if either is missing.
func section(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	s = s[i:]
	if j := strings.Index(s, to); j > 0 {
		return s[:j]
	}
	return s
}
