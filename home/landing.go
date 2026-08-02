package home

import (
	"net/http"

	"mu/internal/app"
)

// Landing is the front door for anyone not signed in: what this is, what an
// agent gets, and what a call costs.
//
// It used to be three pages. The live home was the front door and said nothing
// about what Mu is; /about carried the pitch; /agents carried a second pitch
// plus the payment explanation, and /tools linked to it — so finding out what
// this place is took three hops through pages that each looked like a landing.
// They are one page now, and /about and /agents redirect here.
func Landing(w http.ResponseWriter, r *http.Request) {
	body := `<p class="lead">The everyday internet as tools an agent can call — news, web search,
mail, markets, weather, video, storage. Over <a href="/mcp">MCP</a> and REST, paid per request
in USDC. No API key, no account, no signup.</p>

<div class="lctas">
  <a class="lcta" href="/tools">Browse the tools →</a>
  <a class="lcta lcta-alt" href="/mcp">MCP endpoint</a>
  <a class="lcta lcta-alt" href="/home">See it working</a>
</div>

<div class="lcards">
  <div class="lcard"><h3>Real tools, not wrappers</h3><p>Most things offering an agent tools are a thin layer over somebody else's API. This instance runs the mail server, the feeds, the search index and the sandbox.</p></div>
  <div class="lcard"><h3>Two doors, one set of tools</h3><p>An agent calls <code>/mcp</code>. A person signs in and gets the home screen. Same services underneath — nothing is built twice.</p></div>
  <div class="lcard"><h3>Yours to run</h3><p>One Go binary, self-hostable. Run an instance and you are the operator — anyone paying to call your tools pays you.</p></div>
</div>

<div class="lpay" id="paying">
  <h2>How paying works</h2>
  <ol>
    <li><b>No login to call.</b> Point an agent or MCP client at an endpoint. Your first 10 calls per wallet are free.</li>
    <li><b>When payment is due,</b> the endpoint answers <code>HTTP 402</code> with a price. Your agent's
    <a href="https://x402.org">x402</a> wallet pays in stablecoin (USDC) and retries — sub-second, no account, no keys.</li>
    <li><b>You pay the operator</b> running this instance, directly, wallet to wallet. No middleman.</li>
  </ol>
  <p class="lnote">Prefer prepaid credits, a dashboard and the full app?
  <a href="/signup">Create an account →</a> Both reach the same tools.</p>
</div>

<style>
.lead{max-width:600px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 26px}
.lead a{color:#111}
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

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — Tools for Agents",
		Description: "The everyday internet as tools an agent can call — news, web search, mail, markets, weather, video, storage. Over MCP and REST, paid per request. Open source and self-hostable.",
		Brand:       "Mu",
		Tagline:     "Tools for Agents",
		TopRight:    `<a href="/login">Sign in →</a>`,
		Body:        body,
		Footer:      app.FooterLinks(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}
