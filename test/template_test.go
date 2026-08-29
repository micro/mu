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

// The envelope in the header opens the inbox.
//
// It pointed at /mail for as long as it existed, and /mail is the other thing —
// the envelopes SMTP delivered, not the conversations. On a phone the sidebar is
// behind a hamburger, so this badge is the only sign anything arrived, and it
// took the reader to the wrong page.
func TestTheHeaderEnvelopeOpensTheInbox(t *testing.T) {
	out := app.RenderHTML("A page", "a description", "<p>body</p>",
		&auth.Account{ID: "someone", Name: "Someone"})

	head := section(out, `<div id="head-right"`, `</div>
    </div>`)
	if head == "" {
		t.Fatal("no header cluster in the rendered shell")
	}
	if !strings.Contains(head, `href="/inbox"`) {
		t.Errorf("the header envelope does not open the inbox:\n%s", head)
	}
	if strings.Contains(head, `href="/mail"`) {
		t.Errorf("the header envelope opens the mail store:\n%s", head)
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
