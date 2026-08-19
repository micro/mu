package home

import (
	"html"
	"net/http"
	"strconv"

	"mu/internal/api"
	"mu/internal/app"
	"mu/service/mail"
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
	body := landingBody(app.BaseURL(r))

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — A personal agent",
		Description: "Give your agent an address. Write to it and it answers — with news, web search, mail, markets, weather, places and storage behind it. Open source and self-hostable.",
		Brand:       "Mu",
		// The tagline and the headline in the body were two different pitches
		// stacked: "An Inbox for Agents" over "A personal agent." The first was
		// the line this positioning replaced and it survived here because
		// nothing renders the two together except the page.
		Tagline:  "A personal agent",
		TopRight: `<a href="/login">Sign in →</a>`,
		Body:     body,
		Footer:   app.FooterLinks(),
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
// What is left: a headline, a sentence, the address, two ways on.
//
// The prose explaining all that is here rather than in the stylesheet, because
// a comment inside the <style> block is served to every visitor — which is how
// a test looking for "Connect via MCP" on the page found it in the note saying
// the section had been removed.
func landingBody(host string) string {
	// Counted, not claimed. This said "67 real tools" as a literal and the
	// endpoint was serving 72 by the time anyone checked.
	// The seven is the list immediately before it, counted. Change one and change
	// the other — the number is what makes the claim land, because the reader has
	// just read the things it counts.
	// The address first, and the box below it as proof.
	//
	// This led with the tools and a chat box, which is the right way round for
	// "tools for agents" and the wrong way round for what is actually
	// differentiated. Every provider ships an MCP server now; what none of them
	// ship is an agent that is permanently reachable and remembers. An address
	// is the only handle that needs nothing on the other side — no SDK, no
	// OAuth, no protocol to adopt — so a person can use it, another agent can
	// use it, a form can use it.
	//
	// The box stays, underneath. It is the only thing on the page that shows the
	// product working in five seconds, and a page that only describes an address
	// is asking somebody to imagine what happens when they write to it.
	addr := mail.SharedAgentAddress()
	if addr == "" {
		addr = "agent@" + host
	}
	return `<div class="lwrap">
<h2 class="lhead">A personal agent.</h2>
<p class="lead">It has an email address. Write to it and it answers — with
` + strconv.Itoa(api.ToolCount()) + ` tools behind it: news, web search, mail, markets,
weather, places, storage. One account instead of seven providers.</p>

<div class="laddr"><code>` + html.EscapeString(addr) + `</code></div>
<p class="laddrnote">That one reaches the default agent. Your own get their own —
<code>you+research@</code> — and each answers in the thread, remembers the last
one, and can be reached from anywhere that can send an email.</p>

<div class="lctas">
  <a class="lcta" href="/signup">Get an agent →</a>
  <a class="lcta lcta-alt" href="/tools">Browse the tools</a>
</div>
</div>

<style>
.lwrap{min-height:calc(100vh - 200px);display:flex;flex-direction:column;justify-content:center;padding:20px 0}
.lhead{max-width:700px;text-align:center;margin:0 auto 12px;font-size:38px;line-height:1.12;
  letter-spacing:-.02em;font-weight:700;color:#111}
.lead{max-width:560px;text-align:center;color:#555;font-size:17px;line-height:1.6;margin:0 auto 26px}
.lead a{color:#111}
.laddr{text-align:center;margin:0 auto 10px}
.laddr code{display:inline-block;background:#111;color:#fff;border-radius:8px;padding:12px 20px;
  font-size:18px;letter-spacing:.01em;overflow-wrap:anywhere}
.laddrnote{max-width:560px;margin:0 auto 30px;text-align:center;font-size:13px;color:#888;line-height:1.55}
.laddrnote code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.95em}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:0}
.lcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:12px 24px;border-radius:8px;font-weight:700;font-size:15px}
.lcta-alt{background:#fff;color:#111;border:1px solid #ddd}
.lcta-alt:hover{border-color:#bbb}
@media (max-width:640px){.lhead{font-size:28px}.lead{font-size:15px}.lwrap{min-height:0}}
</style>`
}
