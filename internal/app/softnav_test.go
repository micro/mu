package app

// What a soft navigation has to replace, and what it must not.
//
// A click is answered by fetching the page and swapping #content, which is why
// there is no flash. Everything outside #content stays as the last full page
// load left it — and one of those things decides the width of the page.
//
// body.page-home #content is 1700px where every other page is 1400. Load /apps,
// click Home, and Home laid itself out under Apps' rules until you refreshed;
// the width appeared to follow you between pages. Nothing was stale — the
// selector was matching the wrong page, because body's class came from a
// document that had been replaced.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// adminAccount is an admin, for the one assertion that needs the link drawn.
func adminAccount() *auth.Account { return &auth.Account{ID: "boss", Admin: true} }

// script is the shell's inline JavaScript, which is where the swap lives.
func script(t *testing.T) string {
	t.Helper()
	page := renderWithLang("t", "d", "", "en", nil)
	i := strings.Index(page, "function swap(")
	if i < 0 {
		t.Fatal("no swap() in the shell — soft navigation has moved and this test " +
			"is reading the wrong thing")
	}
	return page[i:]
}

// The swap takes the new document's body class.
func TestASoftNavigationTakesTheNewPagesBodyClass(t *testing.T) {
	body := script(t)
	if !strings.Contains(body, "document.body.className") {
		t.Error("swap() never sets body's class, so every page after the first is " +
			"laid out under the rules of whichever page was loaded for real — " +
			"body.page-home #content is 1700px and everything else is 1400")
	}
	// From the fetched document, not from anywhere else. Reading it off the
	// current body would be the bug written down.
	if !strings.Contains(body, "doc.body") {
		t.Error("body's class is set from something other than the fetched document")
	}
}

// And it keeps the classes no page ever renders.
//
// nav-collapsed is the reader's choice of sidebar, signed-in is added by mu.js
// when the session check returns. Neither is in any server response, so a
// wholesale copy drops both: the sidebar springs open on every click, and every
// soft-navigated page looks signed out until the next check.
func TestASoftNavigationKeepsTheClassesTheServerNeverSends(t *testing.T) {
	body := script(t)
	for _, cls := range []string{"nav-collapsed", "signed-in"} {
		if !strings.Contains(body, "'"+cls+"'") {
			t.Errorf("swap() does not preserve %s, which no page renders and only "+
				"the browser knows", cls)
		}
	}
}

// The admin door is looked up by the id it actually has.
//
// mu.js hides it for a viewer whose session says they are not an admin, which
// is the guard against a page cached for one person being handed to another.
// It moved from the rail to the header and was renamed with it; a lookup left
// pointing at the old id finds nothing and silently guards nothing.
func TestTheAdminLinkIsFoundByTheIdItHas(t *testing.T) {
	b, err := htmlFiles.ReadFile("html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if strings.Contains(js, `getElementById("nav-admin")`) {
		t.Error("mu.js still looks for nav-admin; the link is head-admin now, so " +
			"the lookup returns nothing and the check it guards never runs")
	}
	if !strings.Contains(js, `getElementById("head-admin")`) {
		t.Error("nothing in mu.js finds the admin link, so a page cached for an " +
			"admin keeps offering the door to whoever it is served to next")
	}
	// And the id really is what the shell renders.
	page := renderWithLang("t", "d", "", "en", nil)
	_ = page
	if !strings.Contains(headAdmin(adminAccount()), `id="head-admin"`) {
		t.Error("headAdmin does not render id=head-admin, so mu.js is looking for " +
			"something that is not there")
	}
}

// The loading dim waits, so quick navigations never show one.
//
// It was applied the moment a click landed. That made every navigation flicker,
// and made one of them look broken: an app page is a standalone document with
// no #content — a frame around untrusted HTML, deliberately not part of the
// shell — so opening an app dimmed the page, found nothing to swap, and then
// hard-navigated. Dim, then reload, for a page that was always going to be a
// full load.
func TestTheLoadingDimIsDelayed(t *testing.T) {
	body := script(t)
	i := strings.Index(body, "data-loading")
	if i < 0 {
		t.Fatal("nothing sets data-loading any more")
	}
	// Set behind a timer rather than inline on the click.
	if !strings.Contains(body, "setTimeout(function(){ content.setAttribute('data-loading'") {
		t.Error("the dim is applied immediately, so every navigation flickers and " +
			"a full-page one dims before it reloads")
	}
	if !strings.Contains(body, "clearTimeout(dim)") {
		t.Error("the pending dim is never cancelled, so a fast response still dims " +
			"after it has already been swapped in")
	}
}

// And a response with nothing to swap hands over without leaving the dim on.
func TestAFullPageResponseDoesNotStayDimmed(t *testing.T) {
	body := script(t)
	i := strings.Index(body, "if (!next)")
	if i < 0 {
		t.Fatal("swap() no longer handles a response with no #content")
	}
	branch := body[i:]
	if j := strings.Index(branch, "\n"); j > 0 {
		branch = branch[:j]
	}
	if !strings.Contains(branch, "removeAttribute('data-loading')") {
		t.Errorf("the no-#content branch hard-navigates without clearing the dim, "+
			"so the page it leaves is dimmed on the way out: %s", branch)
	}
}
