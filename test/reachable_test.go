package test

// A page nobody signed in can reach is a page that does not exist.
//
// The footer — About, Tools, Privacy, Status — is not rendered once you are
// signed in. That is a deliberate design decision and a good one: a marketing
// nav under every app screen is the clearest tell that this is a website rather
// than a product. It was justified in a comment saying everything in the footer
// is in the sidebar or on /account.
//
// It was not. Tools was in the sidebar and the rest were nowhere, so somebody
// with an account could not reach the pricing page, the help page or the API
// reference from anywhere in the product. Support had already been noticed and
// patched into the sidebar on its own, one link at a time, which is what this
// failing quietly looks like: it gets fixed for whichever link somebody happens
// to miss, and the next page added behind the footer disappears again.
//
// Help and Support have since been deleted rather than fixed, which is the
// other way to make a page reachable.
//
// So this holds the claim rather than the layout. Where a link lives is a design
// question; that a signed-in account can get to it at all is not.

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
)

var footerHref = regexp.MustCompile(`href="(/[a-z0-9/-]*)"`)

// Account screens deliberately omit marketing navigation. Browser tests cover
// the rendered Go shell; public footer pages remain directly accessible.
func TestAccountDoesNotEmbedTheLandingFooter(t *testing.T) {
	page := app.RenderHTML("Settings", "", "", &auth.Account{ID: "reader"})
	if strings.Contains(page, `id="footer"`) {
		t.Fatal("landing footer returned to signed-in shell")
	}
}

// And the two reference pages are reachable from the one page somebody opens in
// order to point something at this instance.
//
// /account is where they are guaranteed to be findable; /tools is where they are
// wanted. A developer looking for the API docs does not think "settings".
func TestTheReferencePagesAreLinkedFromTools(t *testing.T) {
	registerAll(t)
	loadTools(t)

	// The rendered page, not the source. A link written into a branch that
	// never runs is in the file and not on the screen.
	w := httptest.NewRecorder()
	api.ToolsPageHandler(w, httptest.NewRequest("GET", "/tools", nil))
	page := w.Body.String()

	for _, href := range []string{`href="/api"`, `href="/mcp"`} {
		if !strings.Contains(page, href) {
			t.Errorf("/tools does not link to %s — it is the page somebody opens to "+
				"connect something, and the reference for how is somewhere else", href)
		}
	}
}
