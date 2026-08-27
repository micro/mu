package home

import (
	"net/http"
	"strconv"

	"mu/internal/api"
	"mu/internal/app"
)

// tools is how many there are, rounded down to something a person reads.
//
// Still counted rather than claimed, which is the rule the exact number was
// there to keep: this page once said "67 real tools" as a literal while the
// endpoint served 72. Rounding down cannot overstate — 112 reads as "100+" and
// 210 as "200+" — so the claim stays true without a number that changes under
// the reader for no reason they can see. An exact count is a fact nobody
// wanted; what it is doing on a landing page is saying "a lot", and it should
// say that.
//
// Under a hundred it says the number, because "0+" is not a claim and a small
// instance rounding to nothing would be worse than the truth.
func tools() string {
	n := api.ToolCount()
	if n < 100 {
		return strconv.Itoa(n)
	}
	return strconv.Itoa(n/100*100) + "+"
}

// Landing is the front door for anyone not signed in: something to try, then
// what this is and how to connect to it.
//
// It used to be three pages. The live home was the front door and said nothing
// about what Mu is; /about carried the pitch; /agents carried a second pitch
// plus the payment explanation, and /tools linked to it — so finding out what
// this place is took three hops through pages that each looked like a landing.
// They are one page now, and /about and /agents redirect here.
//
// And then it was one page that only described things. Three buttons, three
// cards and a setup guide: everything about the tools, nothing you could do
// with them. "See it working" was a link, which is a promise rather than a
// demonstration.
//
// The model to copy is ollama. Its product is running models locally, and what
// makes that land is `ollama run llama3` putting you in a conversation on the
// first command — because nobody wants a model, they want to use one. Nobody
// wants tools either. So the tools are used here, on arrival, by anyone: a
// guest gets a few agent queries a day (agent/guest.go) against every service
// that is not somebody's private data, which is enough to watch it read the
// news, check a market and answer.
//
// The copy used to name the number — "three questions a day without an
// account" — and that was the wrong promise to make. There is no free plan
// here, and a page offering a daily allowance is describing one; it also fixed
// in writing a number that is an operator's to change, and made a spend that
// scales with arrivals sound like a feature. What this is, and what it now
// says, is a demonstration: enough to see it work, bounded by a ceiling the
// operator sets, and not a tier anybody can plan around.
//
// The description still follows. It reads better as a caption than as a pitch.
func Index(w http.ResponseWriter, r *http.Request) {
	body := indexBody()

	page := app.RenderIndex(app.Index{
		// What it is, not what to think of it. This said "A network for
		// humans, agents and services" with a paragraph of positioning under
		// it — a claim a stranger is invited to weigh, which is a landing
		// page's job and not a server's.
		Title:       "Mu",
		Description: "A personal server: mail, chat, files, an inbox with an address, and an agent that reaches its tools. Open source and self-hostable.",
		Brand:       "Mu",
		// No tagline in the chrome. This slot held "An Inbox for Agents" — the
		// line this positioning replaced — sitting directly above a headline
		// that said something else, and swapping it for the new line only made
		// the page say the headline twice in three centimetres. The
		// headline is where the line belongs: it is set at 38px and the tagline
		// slot is 18px, so the chrome copy was a smaller, duplicate version of
		// the thing immediately below it. Nothing renders the two together
		// except the page, which is why neither reading caught it.
		// Two controls, both of them things you do rather than things to
		// consider. Install ships hidden: the browser decides whether a site
		// can be installed and says so by firing beforeinstallprompt, and a
		// button that does nothing in Firefox is worse than no button. See
		// installScript.
		//
		// There is no Get started. Signing up is a thing this instance may or
		// may not allow — and "Get started" is the phrase for persuading a
		// stranger, which is not what a server's front door is for. Sign in is
		// the door; whoever runs this decides who gets a key.
		TopRight: `<a href="/login">Sign in →</a>` +
			`<button type="button" id="install-app" hidden>Install</button>` +
			`<span id="install-how" hidden>Share, then Add to Home Screen</span>`,
		Body:   body,
		Footer: app.FooterLinks(),
		Tail:   installScript(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page)) //nolint:errcheck
}

// indexBody is the page itself, separate from serving it.
//
// Separated so it can be read by a test, which is how the duplicated guest
// note went unnoticed: nothing could look at the whole page at once, so two
// paragraphs saying the same thing stayed correct individually and wrong
// together.
// One screen, and it fits on one.
//
// This page had a chat box, three feature cards and a four-step section on
// pointing an MCP client at the endpoint — a landing that argued for four
// things and scrolled for two of them. None of the parts were wrong; each was
// true and each argued for something different. A visitor here is deciding
// whether they want an agent at all, which is one question, and how to
// configure Cursor is a question asked by somebody who already decided. That
// one belongs on /tools, which is the page about tools.
//
// What is left: a headline, a sentence, the address, one way on.
//
// There were two buttons and the second kept being the wrong one. It was
// "Browse the tools", which answers a question a first-time visitor has not
// asked; then "Browse the agents", which sends somebody signed out to a page
// that lists the agents they have not made. A second button is a choice, and
// there was only one thing to do here.
//
// Install app is the exception, and the reason is the thing both of those got
// wrong: they were other destinations, so the page asked you to pick where to
// go. This one is not a destination at all — it is the same page, made
// resident — and it appears only when the browser has told us it can be
// installed, so on a browser that cannot it is not a choice on the page at
// all. See installScript.
//
// And it has to actually fit, which is a measurement rather than an intention.
// The first version of this centred the block with min-height:calc(100vh -
// 200px), forgetting that the shell it sits in already pads 14vh from the top
// and that there is a wordmark above and a footer below — so the page asked for
// a full screen inside something that had already spent a fifth of one, and
// scrolled by 74px at 1440x900 while claiming to be one screen. The block sizes
// to its content now and the shell does the positioning.
//
// Not centred in the leftover space either, which was the next thing tried: the
// shell pads 14vh before the wordmark, so centring what comes after it leaves
// the wordmark alone at the top with a hole under it. The group reads as one
// thing when it flows, and the whitespace goes at the bottom where a landing
// normally puts it.
//
// The prose explaining all that is here rather than in the stylesheet, because
// a comment inside the <style> block is served to every visitor — which is how
// a test looking for "Connect via MCP" on the page found it in the note saying
// the section had been removed.
// No address on the page.
//
// It has been through every position: the largest element on the page, then a
// retyping animation, then a static line under the button. Each move was an
// attempt to say "and you can email it too" without the page becoming about
// email. The move that works is not making it smaller — it is that a landing
// gets one call to action, and an address printed beside a button is a second
// one that nobody signed out can use. You cannot write to you+agent@ until
// there is a you.
//
// The lead still says you can write to it from anywhere, which is the fact. The
// address is on the pages where it is actionable — /agents, Connect, the inbox.
// indexBody is the signed-out page: a search box.
//
// # Why there is no pitch here any more
//
// This was a landing page — a headline, a paragraph of positioning, Get
// started, and "The agent is free. Tools that cost us are priced." That last
// line is our business model, and it shipped in every binary: on somebody
// else's server they pay the vendor themselves, so "costs us" is us, in their
// house, describing an arrangement they are not part of.
//
// The general fault is bigger than that line. A landing page is for a stranger
// who has not decided yet, which is a real audience for micro.mu and nobody at
// all on an instance that one person installed and is already logged into on
// their other device. Marketing is a thing an instance may serve; it is not a
// thing the software should contain.
//
// # A search box, because the archive is public
//
// What is useful without an account is what this instance has collected — and
// /archive is already public by construction: an entry with an owner is never
// returned from it, so there is nothing here to leak and nothing to gate. The
// box is the same one Home shows when no model is configured, pointed at the
// same page, for the same reason: you type what you are looking for and
// something finds it.
//
// That is also the whole of what a utility's front door is. nginx's default
// page says it is nginx and that it is working. This says what it is, that it
// is running, and gives you something to do with it.
func indexBody() string {
	return `<div class="lwrap">
<form class="lsearch" method="GET" action="/archive">
  <input class="lsearch-in" type="search" name="q" placeholder="Search everything here" maxlength="256" autofocus>
  <button class="lsearch-go" type="submit" aria-label="Search">&#x2192;</button>
</form>
<p class="lwhat">` + tools() + ` tools, an inbox with an address, and an agent that reaches them.
<a href="/tools">See what it runs</a></p>
</div>

<style>
.lwrap{padding:0;max-width:560px;margin:0 auto;width:100%}
.lsearch{display:flex;align-items:center;gap:8px;width:100%}
.lsearch-in{flex:1;min-width:0;font:inherit;font-size:16px;padding:12px 14px;border:1px solid #ddd;
  border-radius:var(--border-radius,6px);background:#fff;color:#111}
.lsearch-in:focus{outline:none;border-color:#999}
/* An arrow, the same control the ask box on Home uses. A word here would be
   the only labelled button in the product's two search boxes, and "Search"
   beside a box that says "Search everything here" is the label twice. */
.lsearch-go{flex:none;border:0;background:#111;color:#fff;font:inherit;font-size:16px;
  width:44px;height:44px;border-radius:var(--border-radius,6px);cursor:pointer}
.lwhat{text-align:center;color:#888;font-size:13px;line-height:1.6;margin:16px auto 0}
.lwhat a{color:#555;font-weight:600;text-decoration:none;white-space:nowrap}
.lwhat a:hover{text-decoration:underline}
/* Not stacked on a phone: the box and its arrow are one control, and a
   full-width button under a full-width input reads as two. */
@media (max-width:640px){.lsearch-in{font-size:16px}}
</style>`
}

// installScript makes the Install app button work, and decides whether it is
// there at all.
//
// A web app is installed by the browser, not by the page. Chromium browsers
// announce that they are willing by firing beforeinstallprompt, which is the
// only signal there is — so the button ships hidden and appears when that
// arrives. A button that is always on the page would do nothing in Firefox and
// nothing in an installed window, and a control that does nothing is worse than
// no control.
//
// iOS is the exception worth handling. There is no API at all — the only way in
// is Share, then Add to Home Screen — so on an iPhone the button says where
// that is instead of pretending it can do it.
//
// It registers the service worker too. The app shell does that and this page
// does not, and a browser will not offer to install a site that has none — so
// the first page a visitor sees was the one page that could never be installed
// from.
func installScript() string {
	return `<script>
(function () {
  // The worker first, and unconditionally.
  //
  // This sat below a "no install button, nothing to do here" bail, which was
  // fine while the button was always on the page. The button has gone with the
  // pitch it stood next to, and the registration is not about the button: it
  // is what makes this page installable at all, and the comment above this
  // function exists because the first page a visitor sees was once the one
  // page that could never be installed from. Ordering put it back.
  if (navigator.serviceWorker) {
    navigator.serviceWorker.register('/mu.js', {scope: '/'});
  }

  var btn = document.getElementById('install-app');
  if (!btn) return;
  var how = document.getElementById('install-how');

  // Already installed: this is the app, running in its own window.
  if (window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true) return;

  var offer = null;
  window.addEventListener('beforeinstallprompt', function (e) {
    e.preventDefault();
    offer = e;
    btn.hidden = false;
  });
  window.addEventListener('appinstalled', function () {
    offer = null;
    btn.hidden = true;
    if (how) how.hidden = true;
  });

  // An iPad reports itself as a Mac, so the touch points are the tell.
  var ios = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  if (ios && how) btn.hidden = false;

  btn.addEventListener('click', function () {
    if (offer) {
      offer.prompt();
      offer.userChoice.then(function () { offer = null; btn.hidden = true; });
      return;
    }
    if (how) how.hidden = !how.hidden;
  });
})();
</script>`
}
