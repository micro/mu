// Package agents renders Mu's API face at /agents: the same services, presented
// for machine consumers — every capability as an MCP tool and REST endpoint,
// pay-per-call over x402. It's a logged-out front door on the same backend, not
// a separate product.
package agents

import (
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
)

// canonical returns the base URL for sign-in/account links. Configured via
// APP_URL; empty means same-origin relative links (the single-domain default).
func canonical() string {
	return strings.TrimRight(settings.Get("APP_URL"), "/")
}

// Handler renders the agents / API landing page.
func Handler(w http.ResponseWriter, r *http.Request) {
	base := canonical() // "" => same-origin

	body := `<p style="max-width:560px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 28px">
Every capability, as a service your agents can call — news, markets, weather, web search,
video, mail, your own database and more. Reachable over <a href="/mcp" style="color:#111">MCP</a> and REST, paid
per request with <a href="https://x402.org" style="color:#111">x402</a> stablecoin micropayments.</p>

<div class="pcards">
  <a class="pcard" href="/mcp"><h3>MCP endpoint</h3><p>Point any MCP client here to use every tool. <code>/mcp</code></p></a>
  <a class="pcard" href="/api"><h3>REST &amp; API docs</h3><p>The same services over plain HTTP, with schemas.</p></a>
  <a class="pcard" href="https://x402.org"><h3>Pay per call (x402)</h3><p>Micropayments over HTTP 402. First calls free.</p></a>
</div>

<div class="px402">
  <h2>How paying works</h2>
  <ol>
    <li><b>No login to call.</b> Point an agent or MCP client at an endpoint. Your first calls are free.</li>
    <li><b>When payment is due,</b> the endpoint answers <code>HTTP 402</code> with a price. Your agent's
    <a href="https://x402.org">x402</a> wallet pays in stablecoin (USDC) and retries — sub-second, no account, no keys.</li>
    <li><b>You pay the operator</b> running this instance, directly, wallet-to-wallet. No middleman.</li>
  </ol>
  <p class="pnote">Prefer prepaid credits and a dashboard instead of a wallet?
  <a href="` + base + `/signup">Create an account →</a> — one account for the whole platform.</p>
</div>

<div style="margin-top:26px"><a class="pcta" href="` + base + `/signup">Create an account &amp; get keys →</a></div>

<style>
.pcards{display:flex;flex-wrap:wrap;gap:14px;max-width:760px;justify-content:center}
.pcard{flex:1 1 220px;min-width:220px;max-width:240px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;text-decoration:none;color:inherit;background:#fff;transition:border-color .15s,box-shadow .15s;text-align:left}
.pcard:hover{border-color:#bbb;box-shadow:0 2px 10px rgba(0,0,0,.05)}
.pcard h3{margin:0 0 6px;font-size:1em}
.pcard p{margin:0;font-size:14px;color:#666}
.pcard code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.px402{max-width:620px;margin:44px auto 0;text-align:left;border-top:1px solid #eee;padding-top:28px}
.px402 h2{font-size:1.15em;margin:0 0 12px}
.px402 ol{margin:0 0 14px;padding-left:20px}
.px402 li{margin:0 0 10px;font-size:15px;line-height:1.55;color:#333}
.px402 code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.pnote{font-size:14px;color:#666;margin:0}
.pcta{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:11px 20px;border-radius:8px;font-weight:700;font-size:15px}
</style>`

	page := app.RenderLanding(app.Landing{
		Title:       "Mu — APIs for agents",
		Description: "Every Mu capability as an MCP tool and REST API, paid per call over x402.",
		Brand:       "Mu",
		Tagline:     "APIs for agents",
		TopRight:    `<a href="` + base + `/login">Sign in →</a>`,
		Body:        body,
		Footer: `<a href="/mcp">MCP</a>
  <a href="/api">API</a>
  <a href="/docs">Docs</a>
  <a href="https://x402.org">x402</a>
  <a href="` + base + `/login">Sign in</a>`,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}
