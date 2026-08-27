package home

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/app"
)

// The first screen is about one thing.
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
// This used to say "and there is nothing below it", and now there is: a band
// for somebody who already has an agent — see developerBand. The rule it was
// protecting survives the change, because the rule was never about length. What
// must not come back is a second thing competing for the *first* screen. Below
// the fold is a different situation: anybody reading there has scrolled past
// the button, which means they did not want it.
//
// So the markers below stay banned on the first screen. They are the parts that
// teach and the parts that pitch a rail; below the fold, x402 is named on
// purpose, because a reader who scrolled past the button wants to know how an
// agent pays without an account.
func TestTheLandingIsASearchBox(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	// The thing somebody with no account can actually do. /archive is public by
	// construction — an entry with an owner is never returned from it — so there
	// is nothing here to gate and nothing to leak.
	if !strings.Contains(body, `action="/archive"`) || !strings.Contains(body, `name="q"`) {
		t.Error("the signed-out page does not offer a search")
	}
	if !strings.Contains(body, `method="GET"`) {
		t.Error("a search that is a POST cannot be linked, bookmarked or gone back to")
	}
	// And the way in, which is the only other thing a stranger needs.
	if !strings.Contains(body, `href="/login"`) {
		t.Error("no way to sign in from the signed-out page")
	}

	// What must not come back. Each is an argument aimed at somebody choosing
	// between products, on a page that — on almost every instance there will
	// ever be — is served by a server one person installed and is already
	// signed into somewhere else.
	for _, gone := range []string{
		"The agent is free",
		"cost us are priced",
		"A network for humans, agents and services",
		"Get started",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("the signed-out page still carries %q", gone)
		}
	}
}

// The line is said once, and the retired ones are not said at all.
//
// The page carried "An Inbox for Agents" in the chrome and its headline below
// it — two different pitches stacked, the older one surviving in the one place
// nothing rendered next to the other. Retiring a line by writing the new one
// into the same slot fixed the contradiction and left the duplication: the same
// sentence twice, once at 18px and once at 38px, on a page whose whole argument
// is that it says one thing.
//
// So the rule is a count, not an absence — an absent old line is what an
// earlier version of this checked, and it passed the whole time the page said
// the new one twice. Both halves are here now. The <head> is excluded on
// purpose: the <title> and the og description are allowed to repeat the pitch,
// because nobody reads them beside it.
func TestThereIsNoTagline(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	page := rec.Body.String()

	// This held the tagline to exactly one appearance, because it was in the
	// chrome and in the headline and read as the same sentence twice in a
	// smaller font. It is zero now: a tagline is a claim a reader is invited to
	// weigh, and a server's front door has nothing to argue.
	for _, gone := range []string{
		"A network for humans, agents and services",
		"An Inbox for Agents",
		"The agent is free",
		"cost us are priced",
	} {
		if strings.Contains(page, gone) {
			t.Errorf("the signed-out page still carries %q", gone)
		}
	}
}

// Install app is on the page, and hidden until the browser says it can.
//
// The browser decides whether a site can be installed and announces it by
// firing beforeinstallprompt. Nothing on the page can ask. So the button ships
// with the hidden attribute and the script reveals it — and the attribute has
// to survive the stylesheet, because .lcta sets display:inline-block and an
// author rule beats the browser's own [hidden]{display:none} whatever its
// specificity. Without the override the button is on the page in Firefox, where
// pressing it does nothing at all.
func TestTheServiceWorkerStillRegistersWithNoInstallButton(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	if !strings.Contains(body, "serviceWorker.register('/mu.js'") {
		t.Fatal("the signed-out page no longer registers the service worker, so the " +
			"first page a visitor sees is the one page that cannot be installed " +
			"from — the exact bug the script was written to fix")
	}
	// Ordering, which is the whole hazard. The registration sat below a bail on
	// the install button being absent, and the button has gone with the pitch it
	// stood next to. A guard above it puts the bug back with nothing to notice.
	reg := strings.Index(body, "serviceWorker.register('/mu.js'")
	if bail := strings.Index(body, "if (!btn) return"); bail >= 0 && bail < reg {
		t.Error("the service worker registration is behind a check for an install " +
			"button that is not on the page")
	}
}

// The footer carries the destinations, because nothing above it does any more.
//
// This was TestTheDeveloperBandIsBelowTheCallToAction, guarding the ordering of
// a "Tools for Agents" band under the hero: the endpoint, four facts about MCP
// and x402, and a button to /tools. Three attempts at a developer pitch have
// now been made on this page and all three came out. The band was the careful
// version — below the fold, no protocol names, one link — and it was still a
// second thing to read on a page whose whole design is one screen with one
// thing to do.
//
// What it uniquely carried was the endpoint, and that is on /tools and /mcp,
// both of which the footer links. So the property left to hold is that the
// footer still does.
func TestTheFooterCarriesTheDestinations(t *testing.T) {
	for _, want := range []string{`href="/tools"`, `href="/api"`} {
		if !strings.Contains(app.FooterLinks(), want) {
			t.Errorf("the footer does not carry %s", want)
		}
	}
}

// And the landing has one call to action, with nothing after it.
func TestTheLandingOffersOneThing(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `action="/archive"`) {
		t.Fatal("the signed-out page lost the one thing it does")
	}
	if strings.Contains(body, `class="dev"`) {
		t.Error("the developer band is back — four attempts at a developer " +
			"pitch on this page have now been removed; the endpoint is on /tools")
	}
}

// The pricing page is gone; /pricing redirects to /tools.
//
// Two tests lived here — that the page was a price list and not a plan
// chooser, and that it said what a mailbox costs. Both were holding a page
// into existence. A pricing page is a thing SaaS has; this is a utility
// somebody runs, and an operator running it privately charges nobody. What a
// credit is worth is beside the balance at /wallet, what a call costs is on
// the tool at /tools, and the machine-readable list is /wallet/pricing.

// The negative-cap guard went with the page too.
//
// dailyCap lived in pricing.go and turned quota.DailyLimit into a sentence —
// which published "capped at -1 a day" when an operation had no cap, because
// NoLimit is a negative sentinel. Nothing formats that value into prose any
// more; every remaining reader (service/sms, internal/quota/allowance) tests
// against NoLimit before using it.
