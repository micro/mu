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
func TestTheLandingIsOneScreenAboutOneThing(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	// What it is, and what is behind it.
	//
	// "tools behind it" was the wording once and the marker was the wording.
	// The lead is copy and copy gets rewritten; what has to survive a rewrite
	// is that the first screen still names what the agent can reach, so the
	// marker is the list rather than the sentence around it.
	for _, want := range []string{"Work with Agents", "hand it a job", "tools: news, mail"} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing is missing %q", want)
		}
	}
	// One way on. There were two, and the second kept being the wrong one —
	// "Browse the tools" answers a question a first-time visitor has not asked,
	// "Browse the agents" sends somebody signed out to a list of the agents they
	// have not made.
	if !strings.Contains(body, `href="/signup"`) {
		t.Error("the landing has no path to /signup")
	}

	// What the *hero* must not carry. Each was a section arguing for something
	// else on the screen that should argue for one thing.
	//
	// Scoped to the hero rather than the page. x402 is named in the developer
	// band below the fold, deliberately: a reader who has scrolled past the
	// button is being told how to pay without an account, which is a fact they
	// came looking for rather than a rail being pitched at somebody deciding
	// whether they want an agent at all.
	hero := body
	if i := strings.Index(body, `class="dev"`); i > 0 {
		hero = body[:i]
	}
	for what, marker := range map[string]string{
		"a chat box":           "mu-chat-form",
		"feature cards":        `class="lcards"`,
		"the MCP setup steps":  "Connect via MCP",
		"a payment rail pitch": "x402",
		// The address had every position on this page — the largest element, a
		// retyping animation, a line under the button — and each move was an
		// attempt to say "you can email it too" without the page becoming about
		// email. A landing gets one call to action, and an address beside a
		// button is a second one nobody signed out can use: there is no
		// you+agent@ until there is a you.
		"an email address": `class="laddr"`,
	} {
		if strings.Contains(hero, marker) {
			t.Errorf("the first screen still carries %s — that is a second thing "+
				"to decide about, and it belongs further down or on the page "+
				"about it", what)
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
func TestThereIsOneTagline(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	page := rec.Body.String()

	// Every line this positioning has moved past. AGENTS.md keeps the list;
	// this keeps them off the page, because the way they survive is by living
	// somewhere nothing renders beside them.
	for _, gone := range []string{
		"An Inbox for Agents", "A personal agent", "building blocks for life",
	} {
		if strings.Contains(page, gone) {
			t.Errorf("the landing still carries the retired line %q, so a visitor "+
				"reads two different pitches", gone)
		}
	}

	body := page
	if i := strings.Index(page, "<body>"); i >= 0 {
		body = page[i:]
	}
	if n := strings.Count(body, "Work with Agents"); n != 1 {
		t.Errorf("the landing says %q %d times on one screen, want once — it is the "+
			"headline, and a second copy in the chrome above it is the same "+
			"sentence in a smaller font", "Work with Agents", n)
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
func TestTheInstallButtonWaitsToBeOffered(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `id="install-app"`) {
		t.Fatal("no install button on the landing")
	}
	i := strings.Index(body, `id="install-app"`)
	tag := body[strings.LastIndex(body[:i], "<"):]
	if j := strings.Index(tag, ">"); j > 0 {
		tag = tag[:j]
	}
	if !strings.Contains(tag, "hidden") {
		t.Errorf("the install button ships visible: %s", tag)
	}
	if !strings.Contains(body, ".lcta[hidden]") {
		t.Error("nothing stops .lcta{display:inline-block} overriding the hidden " +
			"attribute, so the button shows on browsers that cannot install")
	}

	// It listens for the one signal there is, and it registers the worker: a
	// browser will not offer to install a site that has no service worker, and
	// this page is the only one that never registered it.
	for _, want := range []string{"beforeinstallprompt", "serviceWorker", "appinstalled"} {
		if !strings.Contains(body, want) {
			t.Errorf("the install script does not mention %s", want)
		}
	}

	// The primary action is still signing up.
	if strings.Index(body, `href="/signup"`) > strings.Index(body, `id="install-app"`) {
		t.Error("Install app comes before Get started")
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

	if !strings.Contains(body, `href="/signup"`) {
		t.Fatal("the landing lost its call to action")
	}
	if strings.Contains(body, `class="dev"`) {
		t.Error("the developer band is back — three attempts at a developer " +
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
