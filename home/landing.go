package home

import (
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
)

// Landing renders the logged-out front door: a plain landing page that says
// what Mu is and points at the ways in (sign in, create an account, and the
// /agents API face). Signed-in requests never reach here — the root handler
// funnels them to /home — but we guard anyway so a direct hit on the landing
// still does the right thing.
func Landing(w http.ResponseWriter, r *http.Request) {
	if _, acc := auth.TrySession(r); acc != nil {
		http.Redirect(w, r, "/home", http.StatusFound)
		return
	}

	body := `<p class="lead">One agent for the everyday internet — news, mail, search, weather,
markets and video, all handled by an assistant you just talk to. No feeds to doomscroll,
no ads, no tracking. Open source and self-hostable, so you can run the whole stack yourself.</p>

<div class="lcards">
  <div class="lcard"><h3>One agent, everything</h3><p>Ask for the news, your mail, a price, the forecast. Mu picks the right service and answers — you just talk to it.</p></div>
  <div class="lcard"><h3>Real services, not widgets</h3><p>News, markets, mail, weather, blog, video and search — each a genuine service on go-micro, not a scraped feed.</p></div>
  <div class="lcard"><h3>Yours to run</h3><p>A single Go binary you can self-host. Your account, your data, your instance — no lock-in.</p></div>
</div>

<div class="lctas">
  <a class="lcta" href="/signup">Create your account →</a>
  <a class="lcta lcta-alt" href="/login">Sign in</a>
</div>

<p class="lagents">Building agents or apps? Every capability is also an API.
<a href="/agents">See the agent &amp; developer page →</a></p>

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
		Title:       "Mu — an agent for everyday",
		Description: "One agent for the everyday internet: news, mail, search, weather, markets and video. Open source and self-hostable.",
		Brand:       "Mu",
		Tagline:     "An agent for everyday",
		Body:        body,
		TopRight:    `<a href="/login">Sign in →</a>`,
		Footer: `<a href="/agents">Agents</a>
  <a href="/api">API</a>
  <a href="/docs">Docs</a>
  <a href="/mcp">MCP</a>
  <a href="/login">Sign in</a>`,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}
