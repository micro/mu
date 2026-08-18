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
		Title:       "Mu — An Inbox for Agents",
		Description: "Give your agent an address. Write to it and it answers — with news, web search, mail, markets, weather, places and storage behind it. Open source and self-hostable.",
		Brand:       "Mu",
		Tagline:     "An Inbox for Agents",
		TopRight:    `<a href="/login">Sign in →</a>`,
		Body:        body,
		Footer:      app.FooterLinks(),
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
	return `<h2 class="lhead">A personal agent.</h2>
<p class="lead">It has an email address. Write to it and it answers — with
` + strconv.Itoa(api.ToolCount()) + ` tools behind it: news, web search, mail, markets,
weather, places, storage. One account instead of seven providers.</p>

<div class="laddr"><code>` + html.EscapeString(addr) + `</code></div>
<p class="laddrnote">That one reaches the default agent. Your own get their own —
<code>you+research@</code> — and each answers in the thread, remembers the last
one, and can be reached from anywhere that can send an email.</p>

<div class="ltry">` + chatComponent(true) + `</div>

<p class="ltrynote">Or ask it here — a few questions to try it, no account needed.
<a href="/signup">Sign up</a> for an agent of your own, the address it answers on,
and the endpoint your client can call.</p>

<div class="lctas">
  <a class="lcta" href="/inbox">Talk to an agent →</a>
  <a class="lcta lcta-alt" href="/tools">Browse the tools</a>
</div>

<div class="lcards">
  <div class="lcard"><h3>Reachable by anyone</h3><p>An address needs nothing on the other side — no SDK, no key, no signup. A person, another agent, a form or a cron job can write to yours, and it answers in the thread.</p></div>
  <div class="lcard"><h3>It remembers</h3><p>Every conversation is kept, whichever way it arrived — the address, the web, Discord, Telegram, WhatsApp. One thread of what was said, not five that forget on restart.</p></div>
  <div class="lcard"><h3>One server, not nine</h3><p>News, search, mail and storage usually means a provider each: a signup, a card and a key apiece. Here it is one connection, one balance, one thing to rotate — or self-host the binary and it is yours.</p></div>
</div>

<div class="lpay" id="connecting">
  <h2>Connect via MCP</h2>
  <p class="lnote">Two ways in for a client you sign in with, and a third for an agent
  that has no account at all. Everything the inbox holds comes with it — an agent on the
  other end can list your conversations, read one back and search what was said.</p>
  <ol>
    <li><b>Cursor, and clients with a config file.</b> Create a token at <a href="/token">/token</a>
    and point them at <code>` + host + `/mcp</code> with an
    <code>Authorization: Bearer</code> header.</li>
    <li><b>Claude Desktop.</b> Settings → Connectors → Add custom connector, and paste
    <code>` + host + `/mcp</code>. It opens a browser and asks you to sign in — no token needed.</li>
    <li><b>Then call anything.</b> Calls draw on your credits — most tools are included, the ones
    that cost us money carry a price on <a href="/tools">tools</a>.</li>
    <li><b>Or bring a wallet and skip all of that.</b> Call <code>` + host + `/mcp</code> with no
    credentials. Free tools answer; a priced one replies <code>402</code> naming its price and where
    to send it, and an <a href="https://x402.org" rel="noopener">x402</a> client pays in USDC and
    retries. No signup, no key, no card — the payment is the identity.</li>
  </ol>
  <p class="lnote">No account yet? <a href="/signup">Create one →</a> Full setup on
  <a href="/tools">Tools</a>, protocol detail on the <a href="/mcp">MCP server</a> page.</p>
</div>

<style>
.lhead{max-width:700px;text-align:center;margin:0 auto 10px;font-size:34px;line-height:1.15;
  letter-spacing:-.02em;font-weight:700;color:#111}
.lead{max-width:600px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 18px}
.laddr{text-align:center;margin:0 auto 8px}
.laddr code{display:inline-block;background:#111;color:#fff;border-radius:8px;padding:10px 18px;
  font-size:17px;letter-spacing:.01em;overflow-wrap:anywhere}
.laddrnote{max-width:600px;margin:0 auto 26px;text-align:center;font-size:13px;color:#888;line-height:1.55}
.laddrnote code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.95em}
.ltry{max-width:660px;margin:0 auto 10px;text-align:left}
.ltrynote{max-width:660px;margin:0 auto 26px;text-align:center;font-size:13px;color:#888}
.ltrynote a{color:#111}
.lead a{color:#111}
@media (max-width:640px){.lhead{font-size:27px}}
.lcards{display:flex;flex-wrap:wrap;gap:14px;max-width:760px;justify-content:center;margin:34px auto 0}
.lcard{flex:1 1 220px;min-width:220px;max-width:240px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;background:#fff;text-align:left}
.lcard h3{margin:0 0 6px;font-size:1em}
.lcard p{margin:0;font-size:14px;color:#666;line-height:1.5}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:0}
.lcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:11px 22px;border-radius:8px;font-weight:700;font-size:15px}
.lcta-alt{background:#fff;color:#111;border:1px solid #ddd}
.lcta-alt:hover{border-color:#bbb}
.lpay{max-width:620px;margin:44px auto 0;text-align:left;border-top:1px solid #eee;padding-top:28px}
.lpay h2{font-size:1.15em;margin:0 0 12px}
.lpay ol{margin:0 0 14px;padding-left:20px}
.lpay li{margin:0 0 10px;font-size:15px;line-height:1.55;color:#333}
.lpay code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.lnote{font-size:14px;color:#666;margin:0}
</style>`
}
