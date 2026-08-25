package home

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/quota"
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

// The developer band is under the hero, not in it.
//
// Two earlier attempts at a developer pitch went above the fold and both came
// out: "Browse the tools" answers a question a first-time visitor has not
// asked, and a four-step MCP walkthrough is for somebody who already decided.
// The situation below the fold is the opposite — a reader there has scrolled
// past the button, and the likeliest reason a technical visitor does that is
// that they already have an agent.
//
// So the ordering is the test. Everything the band offers must come after the
// one call to action, or it is the same mistake in a longer page.
func TestTheDeveloperBandIsBelowTheCallToAction(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	signup := strings.Index(body, `href="/signup"`)
	band := strings.Index(body, `class="dev"`)
	if signup < 0 || band < 0 {
		t.Fatalf("landing is missing the CTA or the band (signup %d, band %d)", signup, band)
	}
	if band < signup {
		t.Error("the developer band is drawn before Get started, so it competes " +
			"with the one thing a first-time visitor is being asked to do")
	}

	// What the band gives you is the endpoint, which is the actionable thing and
	// the one fact not available anywhere else on the page.
	if !strings.Contains(body[band:], "/mcp") {
		t.Error("the band does not give the endpoint")
	}

	// And no link row of its own. It ended with Tools · API · Pricing, two of
	// which were already in the footer a few centimetres below and the third of
	// which belongs there. A footer is where a site keeps its destinations; a
	// second copy of most of one is furniture.
	if strings.Contains(body[band:], `class="dev-links"`) {
		t.Error("the developer band has grown its own link row again — those " +
			"destinations belong in the footer, which is right below it")
	}
	for _, want := range []string{`href="/tools"`, `href="/api"`, `href="/pricing"`} {
		if !strings.Contains(app.FooterLinks(), want) {
			t.Errorf("the footer does not carry %s", want)
		}
	}
}

// The price list is reachable without an account.
//
// /pricing redirected to /tools, which carries a price on each of a hundred-odd
// entries — that answers what one call costs and never what this is going to
// cost you. Somebody deciding whether to sign up is asking the second question
// and had nowhere to read the answer.
//
// It is not the plan chooser that was deleted. Plans were rebuilt three times
// and came apart on the same fact every time: a credit is a penny and every
// operation costs what quota.json says, whoever is asking, so there is nothing
// to choose.
func TestPricingIsAPriceListAndNotAPlanChooser(t *testing.T) {
	rec := httptest.NewRecorder()
	PricingHandler(rec, httptest.NewRequest("GET", "/pricing", nil))
	body := rec.Body.String()

	if !strings.Contains(body, "One credit is one penny") {
		t.Error("the pricing page does not say what a credit is")
	}
	// Rendered from quota.json rather than written here, which is the only way a
	// price list stays true. Four hand-maintained tables drifted before.
	if !strings.Contains(body, "stats-table") {
		t.Error("the pricing page carries no cost table")
	}

	// No tiers, and nothing recommended.
	for _, banned := range []string{"Most popular", "Recommended", "/month", "per month"} {
		if strings.Contains(body, banned) {
			t.Errorf("the pricing page is offering a plan again: %q", banned)
		}
	}

	// And it is in the footer, so a signed-out visitor can find it.
	if !strings.Contains(app.FooterLinks(), `href="/pricing"`) {
		t.Error("pricing is not in the footer, so nobody signed out will find it")
	}
}

// The pricing page says what the mailbox costs, and reads the numbers.
//
// The page said "Chatting, email, your inbox, your files: no credits" while
// external_email was charged and capped. That was not a wording problem: it is
// the one operation priced for something other than what it costs to run, so it
// is exactly the one a reader must not be told is free.
//
// It matters more now than it did. IMAP and SMTP mean a person can live in
// their own mail client all day, and the natural question — what does that
// cost — had no answer on the page that exists to answer it.
func TestPricingSaysWhatTheMailboxCosts(t *testing.T) {
	rec := httptest.NewRecorder()
	PricingHandler(rec, httptest.NewRequest("GET", "/pricing", nil))
	body := rec.Body.String()

	// The free half, named, because a reader assumes a mailbox is metered.
	for _, want := range []string{"IMAP", "SMTP", "Receiving costs nothing"} {
		if !strings.Contains(body, want) {
			t.Errorf("the pricing page does not mention %q, so somebody deciding "+
				"whether to point a mail client at this has to guess", want)
		}
	}

	// And the paid half, not described as free.
	if strings.Contains(body, "Chatting, email,") {
		t.Error("the pricing page says email is free while mail leaving the " +
			"instance is charged")
	}
	// One price for sending, wherever it is going — the page has to say so,
	// because a reader who has been told local mail is free will assume it
	// still is.
	if !strings.Contains(body, "wherever it is going") {
		t.Error("the pricing page does not say that sending costs the same " +
			"whether the recipient is here or outside")
	}

	// The number is read from quota.json rather than typed here. A price
	// written into a sentence is a price that drifts.
	if !strings.Contains(body, pence(quota.OpMailSend)) {
		t.Errorf("the mailbox section does not carry the configured price (%s)",
			pence(quota.OpMailSend))
	}
}

// A cap of "none" is not a cap of minus one.
//
// quota.DailyLimit returns NoLimit — a negative — for an operation with no cap,
// and quota.json is an operator's file. Dropping that straight into a sentence
// published "capped at -1 a day".
func TestPricingNeverPublishesANegativeCap(t *testing.T) {
	rec := httptest.NewRecorder()
	PricingHandler(rec, httptest.NewRequest("GET", "/pricing", nil))
	if body := rec.Body.String(); strings.Contains(body, "-1 a day") {
		t.Error("the pricing page publishes a negative daily cap")
	}
	if got := dailyCap("an_operation_with_no_limit"); got != "" {
		t.Errorf("an operation with no cap renders %q, want nothing", got)
	}
}
