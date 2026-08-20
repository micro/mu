package home

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/api"
	"mu/internal/app"
)

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
func Landing(w http.ResponseWriter, r *http.Request) {
	body := landingBody()

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — A personal agent",
		Description: "Chat with it here or write to it from anywhere — with news, web search, mail, markets, weather, places and storage behind it. Open source and self-hostable.",
		Brand:       "Mu",
		// No tagline in the chrome. This slot held "An Inbox for Agents" — the
		// line this positioning replaced — sitting directly above a headline
		// that said something else, and swapping it for the new line only made
		// the page say "A personal agent" twice in three centimetres. The
		// headline is where the line belongs: it is set at 38px and the tagline
		// slot is 18px, so the chrome copy was a smaller, duplicate version of
		// the thing immediately below it. Nothing renders the two together
		// except the page, which is why neither reading caught it.
		TopRight: `<a href="/login">Sign in →</a>`,
		Body:     body,
		Below:    developerBand(app.BaseURL(r)),
		Footer:   app.FooterLinks(),
		Tail:     installScript(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page)) //nolint:errcheck
}

// landingBody is the page itself, separate from serving it.
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
func landingBody() string {
	// The tool count is counted, not claimed. This said "67 real tools" as a
	// literal and the endpoint was serving 72 by the time anyone checked.
	//
	// The sentence used to end "One account instead of seven providers", which
	// is the argument rather than the thing — and an argument invites the reader
	// to weigh it, on a page whose job is to say what this is. It also carried a
	// second number that had to be kept in step with the list beside it by hand.
	// The list is the proof; it does not need a claim after it.
	//
	return `<div class="lwrap">
<h2 class="lhead">A personal agent.</h2>
<p class="lead">Chat with it here or write to it from anywhere. ` +
		strconv.Itoa(api.ToolCount()) + ` tools behind it — news, web search, markets,
weather, places, storage — and it remembers the last conversation.</p>

<div class="lctas">
  <a class="lcta" href="/signup">Get started →</a>
  <button type="button" class="lcta lcta-second" id="install-app" hidden>Install app</button>
</div>
<p class="linstall" id="install-how" hidden>In Safari: Share, then Add to Home Screen.</p>
</div>

<style>
.lwrap{padding:0}
.lhead{max-width:700px;text-align:center;margin:0 auto 12px;font-size:20px;line-height:1.2;
  letter-spacing:-.01em;font-weight:700;color:#111}
.lead{max-width:560px;text-align:center;color:#555;font-size:17px;line-height:1.6;margin:0 auto 22px}
.lead a{color:#111}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:0}
/* An explicit line-height on both, or they are different heights side by side:
   a <button> and an <a> take different defaults, and 3px of it shows. */
.lcta,.lcta:visited{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:12px 24px;
  border-radius:var(--border-radius,6px);font-weight:700;font-size:15px;line-height:17px}
/* The rule above is display:inline-block, and an author rule beats the browser's
   own [hidden]{display:none} whatever its specificity. Without this line the
   install button is on the page for everybody, including the browsers that
   cannot install anything. */
.lcta[hidden],.linstall[hidden]{display:none}
/* Second, and it looks it: the primary action on this page is signing up.
   The outline is a shadow rather than a border so it costs no height — a border
   makes this button 2px taller than the link beside it. */
.lcta-second{background:#fff;color:#111;border:0;box-shadow:inset 0 0 0 1px #ddd;
  cursor:pointer;font-family:inherit;font-size:15px}
.lcta-second:hover{box-shadow:inset 0 0 0 1px #111}
.linstall{text-align:center;color:#666;font-size:14px;margin:12px auto 0}
@media (max-width:640px){.lead{font-size:15px}}
</style>`
}

// developerBand is the section under the fold, for somebody who already has an
// agent and wants to point it at this.
//
// Below rather than beside, and that distinction is the whole design. The hero
// is one screen with one thing to do, because a first-time visitor is deciding
// whether they want an agent at all — and a second call to action there is a
// choice offered to somebody who has not made the first one yet. Two earlier
// attempts at a developer pitch went above the fold and both had to come out.
//
// Under it is a different situation. Anybody reading this has scrolled past the
// button, which means they did not want it, and the most likely reason a
// technical visitor does that is that they already have an agent. This is the
// answer to what they are looking for.
//
// It says what is here and links out. It does not teach: the four-step "point
// your client at this" walkthrough belongs on /tools and was on this page once,
// and no protocol is named in the copy — the endpoint is a URL you can read,
// and the payment rail's name is jargon that means nothing to somebody meeting
// it in a hero. Both are one click away.
func developerBand(base string) string {
	endpoint := html.EscapeString(strings.TrimSuffix(base, "/")) + "/mcp"

	return `<div class="dev">
<h2 class="dev-head">Tools for agents</h2>
<p class="dev-lead">Already have an agent? Point it here. ` +
		strconv.Itoa(api.ToolCount()) + ` tools — news, web search, mail, markets,
weather, places, files — behind one endpoint.</p>
<pre class="dev-endpoint">` + endpoint + `</pre>
<ul class="dev-facts">
  <li>The agent costs nothing. You pay for a tool only when calling it costs a
      third party — and not when this instance already had the answer.</li>
  <li>One account instead of six, or no account at all — an agent can pay per
      request as it calls.</li>
  <li>Every tool is also plain HTTP, so a cron job or a script can use them
      without speaking anything special.</li>
  <li>One Go binary. Run your own and whoever calls your tools pays you.</li>
</ul>
<p class="dev-links">` + app.TextLink("Browse the tools", "/tools") + ` · ` +
		app.TextLink("API reference", "/api") + ` · ` +
		app.TextLink("What it costs", "/pricing") + `</p>
</div>

<style>
/* A band, not a second hero: a rule across the top, ordinary text sizes, and
   the same column width as the copy above it. It has to read as somewhere the
   page continues rather than as a competing offer. */
/* The colour and alignment are stated rather than inherited. This sits in the
   shell's Below slot, which wraps it in .also — centred, 14px, #888, written
   for a one-line "also on Discord" — and anything here that did not set its own
   would come out as small grey centred text. */
.dev{max-width:560px;margin:0 auto;padding:28px 0 0;border-top:1px solid #eee;
  text-align:left;color:#111}
.dev-head{font-size:15px;font-weight:700;color:#111;margin:0 0 8px}
.dev-lead{font-size:15px;color:#555;line-height:1.6;margin:0 0 14px}
.dev-endpoint{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;
  color:#333;background:#f6f6f6;border-radius:6px;padding:10px 12px;margin:0 0 14px;
  overflow-x:auto}
.dev-facts{font-size:14px;color:#555;line-height:1.6;margin:0 0 14px;padding-left:18px}
.dev-facts li{margin:0 0 6px}
/* The landing shell does not load mu.css, so app.TextLink's class means nothing
   here and these came out as browser-default blue and underlined, on a page
   where every other word is #111 or #555. */
.dev-links{font-size:14px;margin:0;color:#888}
.dev-links a{color:#111;text-decoration:none;font-weight:600}
.dev-links a:hover{text-decoration:underline}
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
  var btn = document.getElementById('install-app');
  if (!btn) return;
  var how = document.getElementById('install-how');

  // Already installed: this is the app, running in its own window.
  if (window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true) return;

  if (navigator.serviceWorker) {
    navigator.serviceWorker.register('/mu.js', {scope: '/'});
  }

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
