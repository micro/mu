package home

import (
	"net/http"

	"mu/internal/app"
)

// Landing renders the "what is Mu" pitch, served at /about. The live home is
// the front door (immediate usage drives signups); this page is for visitors
// who want the explanation. Viewable signed-in or out.
func Landing(w http.ResponseWriter, r *http.Request) {
	body := `<p class="lead">The everyday internet as tools an agent can call — news, web search,
mail, markets, weather, video, storage. Over <a href="/mcp" style="color:#111">MCP</a> and REST,
paid per request in USDC. No API key, no account, no signup.</p>

<div class="lcards">
  <div class="lcard"><h3>Real tools, not wrappers</h3><p>This instance runs the mail server, the feeds, the search index and the sandbox. <a href="/tools">Browse the tools →</a></p></div>
  <div class="lcard"><h3>Two doors, one set of tools</h3><p>An agent calls <code>/mcp</code>. A person signs in and gets the home screen. Same services underneath — nothing is built twice.</p></div>
  <div class="lcard"><h3>Yours to run</h3><p>One Go binary, self-hostable. Run an instance and you are the operator — anyone paying to call your tools pays you.</p></div>
</div>

<div class="lctas">
  <a class="lcta" href="/">Open Mu →</a>
  <a class="lcta lcta-alt" href="/tools">Browse the tools</a>
</div>

<p class="lagents">Building agents? Every capability is a tool, over MCP, paid per call.
<a href="/tools">Browse the tools →</a> · <a href="/agents">How it works →</a></p>

<style>
.lead{max-width:560px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 30px}
.lcards{display:flex;flex-wrap:wrap;gap:14px;max-width:760px;justify-content:center;margin:0 auto}
.lcard{flex:1 1 220px;min-width:220px;max-width:240px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;background:#fff;text-align:left}
.lcard h3{margin:0 0 6px;font-size:1em}
.lcard p{margin:0;font-size:14px;color:#666;line-height:1.5}
.lctas{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:34px 0 0}
.lcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:11px 22px;border-radius:8px;font-weight:700;font-size:15px}
.lcta-alt{background:#fff;color:#111;border:1px solid #ddd}
.lcta-alt:hover{border-color:#bbb}
.lagents{margin:34px auto 0;max-width:560px;text-align:center;font-size:14px;color:#888}
.lagents a{color:#555}
</style>`

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — Tools for Agents",
		Description: "The everyday internet as tools an agent can call — news, web search, mail, markets, weather, video, storage. Over MCP and REST, paid per request. Open source and self-hostable.",
		Brand:       "Mu",
		Tagline:     "The everyday internet, yours to run",
		Body:        body,
		TopRight:    `<a href="/login">Sign in →</a>`,
		Footer:      app.FooterLinks(),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}
