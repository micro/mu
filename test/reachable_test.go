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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
)

var footerHref = regexp.MustCompile(`href="(/[a-z0-9/-]*)"`)

func TestEveryFooterLinkIsReachableSignedIn(t *testing.T) {
	links := footerHref.FindAllStringSubmatch(app.FooterLinks(), -1)
	if len(links) < 4 {
		t.Fatalf("found %d footer links — this scan is broken, not the code", len(links))
	}

	// The chrome a signed-in account is served. Rendered rather than asserted
	// about, so a link moving out of the sidebar is noticed here.
	acc := &auth.Account{ID: "reader", Name: "Reader"}
	sidebar := app.RenderHTML("t", "d", "", acc)

	if strings.Contains(sidebar, `id="footer"`) {
		t.Error("the footer is rendered for a signed-in account — if that is now " +
			"intended, this whole test is moot and should go")
	}

	// Anything not in the sidebar has to be on /account, which is. That is one
	// structural fact — the page embeds the footer links — and it is asserted
	// against the source rather than by re-deriving it here, because a helper
	// that returned app.FooterLinks() would be this test agreeing with itself.
	onAccount := accountEmbedsFooterLinks(t)

	for _, m := range links {
		href := m[1]
		if strings.Contains(sidebar, `href="`+href+`"`) {
			continue
		}
		if onAccount {
			continue
		}
		t.Errorf("%s is in the footer, not in the sidebar, and not on /account — "+
			"the footer is not rendered for a signed-in account, so this page is "+
			"unreachable inside the product", href)
	}
}

// accountEmbedsFooterLinks reports whether the account page carries the footer
// links, read from its source.
func accountEmbedsFooterLinks(t *testing.T) bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(at(""), "account/pages.go"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(b), "app.FooterLinks()")
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
