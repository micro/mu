// Package portal renders Mu's developer/API face: the same go-micro services,
// presented for machine consumers — every capability as an MCP tool and REST
// endpoint, pay-per-call over x402, no keys or accounts. It is a face on the
// same backend, not a separate product. Available on every instance at
// /developers, and served at the root of any domain a reverse proxy flags with
// "X-Mu-Portal: developer". The wordmark is derived from the domain. Nothing
// domain-specific is baked in — see docs/DEVELOPER_PORTAL.md.
package portal

import (
	"net/http"

	"mu/internal/app"
)

// Handler renders the developer portal landing.
func Handler(w http.ResponseWriter, r *http.Request) {
	brand := htmlEscape(app.PortalBrand(r))

	content := `<div class="portal">
  <p class="portal-kicker">APIs for agents</p>
  <h1 class="portal-hero">` + brand + `</h1>
  <p class="portal-sub">Every capability, as a service your agents can call — news, markets,
  weather, web search, video, mail and more. Reachable over <a href="/mcp">MCP</a> and REST,
  paid per request with <a href="https://x402.org">x402</a> stablecoin micropayments. No keys,
  no accounts — just call and pay.</p>

  <div class="portal-cards">
    <a class="portal-card" href="/mcp"><h3>MCP endpoint</h3><p>Point any MCP client here to use every tool. <code>/mcp</code></p></a>
    <a class="portal-card" href="/api"><h3>REST &amp; API docs</h3><p>The same services over plain HTTP, with schemas.</p></a>
    <a class="portal-card" href="https://x402.org"><h3>Pay per call (x402)</h3><p>Micropayments over HTTP 402. First calls free.</p></a>
  </div>

  <p class="portal-foot">Built on <a href="https://go-micro.dev">Go Micro</a>. Self-hostable —
  run your own and expose it on your own domain.</p>
</div>

<style>
.portal{max-width:820px;margin:0 auto}
.portal-kicker{text-transform:uppercase;letter-spacing:.08em;font-size:12px;color:#888;margin:0 0 6px}
.portal-hero{font-size:2.6em;font-weight:800;letter-spacing:-.02em;margin:0 0 14px}
.portal-sub{font-size:16px;line-height:1.6;color:#444;margin:0 0 28px}
.portal-cards{display:flex;flex-wrap:wrap;gap:14px;margin:0 0 28px}
.portal-card{flex:1 1 220px;border:1px solid #e5e5e5;border-radius:10px;padding:16px 18px;text-decoration:none;color:inherit;background:#fff;transition:border-color .15s,box-shadow .15s}
.portal-card:hover{border-color:#bbb;box-shadow:0 2px 10px rgba(0,0,0,.05)}
.portal-card h3{margin:0 0 6px;font-size:1em}
.portal-card p{margin:0;font-size:14px;color:#666}
.portal-card code{background:#f4f4f5;border-radius:4px;padding:1px 5px;font-size:.9em}
.portal-foot{font-size:14px;color:#777}
</style>`

	html := app.RenderHTMLForRequest(app.PortalBrand(r)+" — APIs for agents",
		"Every Mu capability as an MCP tool and REST API, paid per call over x402.", content, r)
	w.Write([]byte(html))
}

func htmlEscape(s string) string {
	r := ""
	for _, c := range s {
		switch c {
		case '&':
			r += "&amp;"
		case '<':
			r += "&lt;"
		case '>':
			r += "&gt;"
		default:
			r += string(c)
		}
	}
	return r
}
