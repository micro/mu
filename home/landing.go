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
// What is left: a headline, a sentence, the address, one way on.
//
// There were two buttons and the second kept being the wrong one. It was
// "Browse the tools", which answers a question a first-time visitor has not
// asked; then "Browse the agents", which sends somebody signed out to a page
// that lists the agents they have not made. A second button is a choice, and
// there is only one thing to do here.
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
// No animation on the address.
//
// The word after the plus used to delete and retype itself through research,
// work, family, money, travel — an argument that the word is yours to pick,
// made without a sentence saying so. It was the right trick for a page whose
// thesis was the address. It is not that page any more: the address sits under
// the button now as one fact among several, and a fact that moves while you
// read the rest of the page is asking for attention its position says it does
// not want. A landing gets one thing that moves at most, and this is not it.
func landingBody(host string) string {
	// The tool count is counted, not claimed. This said "67 real tools" as a
	// literal and the endpoint was serving 72 by the time anyone checked.
	//
	// The sentence used to end "One account instead of seven providers", which
	// is the argument rather than the thing — and an argument invites the reader
	// to weigh it, on a page whose job is to say what this is. It also carried a
	// second number that had to be kept in step with the list beside it by hand.
	// The list is the proof; it does not need a claim after it.
	//
	// The address is set as text, not as a black pill. It was drawn with the
	// same background, radius and white-on-#111 as .lcta immediately below it,
	// so the page showed two identical black rectangles of which only the lower
	// one did anything — the address read as the primary button and the primary
	// button read as its twin. It is a thing to read and copy, so it is
	// monospaced and underscored with a hairline instead.
	//
	// The address after the button, not before it.
	//
	// It led the page for a while, on the argument that an address is the only
	// handle needing nothing on the other side — no SDK, no OAuth, no protocol
	// to adopt — which is true and is still why it is here at all. But a page
	// whose largest element is an email address is a page about email, and this
	// is not a mail product. Two of the three things above it said "write to
	// it", so a reader arrived thinking the way in was to compose a message,
	// when the way in for almost everybody is to sign up and type.
	//
	// So the order is what you do first: chat, then the fact that you can also
	// write to it from anywhere. Same address, a third of the weight — and it
	// reads as a detail about the agent rather than the definition of one.
	//
	// It is shown as you+agent@ rather than agent@, and the paragraph explaining
	// the difference has gone with it. That paragraph was three sentences doing
	// the work the address does by itself: agent@ is this instance's shared one,
	// yours are you+something@, and a reader who has to be told that in prose is
	// a reader looking at the wrong address. The form is the explanation.
	domain := mail.ConfiguredDomain()
	if domain == "" {
		domain = host
	}
	return `<div class="lwrap">
<h2 class="lhead">A personal agent.</h2>
<p class="lead">Chat with it here or write to it from anywhere. ` +
		strconv.Itoa(api.ToolCount()) + ` tools behind it — news, web search, markets,
weather, places, storage — and it remembers the last conversation.</p>

<div class="lctas">
  <a class="lcta" href="/signup">Get an agent →</a>
</div>

<div class="laddr"><code>you+agent@` + html.EscapeString(domain) + `</code></div>
</div>

<style>
.lwrap{padding:0}
.lhead{max-width:700px;text-align:center;margin:0 auto 12px;font-size:20px;line-height:1.2;
  letter-spacing:-.01em;font-weight:700;color:#111}
.lead{max-width:560px;text-align:center;color:#555;font-size:17px;line-height:1.6;margin:0 auto 22px}
.lead a{color:#111}
.laddr{text-align:center;margin:26px auto 0}
.laddr code{display:inline-block;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;
  font-size:15px;font-weight:400;color:slategray;letter-spacing:-.01em;
  padding-bottom:6px;border-bottom:1px solid #e8e8ea}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:0}
.lcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:12px 24px;
  border-radius:var(--border-radius,6px);font-weight:700;font-size:15px}
@media (max-width:640px){.lead{font-size:15px}}
</style>`
}
