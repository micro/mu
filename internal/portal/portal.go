// Package portal renders Mu's developer/API face: the same go-micro services,
// presented for machine consumers — every capability as an MCP tool and REST
// endpoint, pay-per-call over x402. It is a logged-out front door on the same
// backend, not a separate product. Account/wallet/keys live on the canonical
// consumer app (APP_URL); API access itself is token/wallet-based, so no
// cross-domain session is needed. The wordmark is derived from the domain.
// Nothing domain-specific is baked in — see docs/DEVELOPER_PORTAL.md.
package portal

import (
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
)

// canonical returns the base URL of the consumer app (where accounts, wallet and
// sign-in live) for links from the portal, which may be on a different domain.
// Configured via APP_URL; empty means same-origin relative links (correct for a
// single-domain self-host).
func canonical() string {
	return strings.TrimRight(settings.Get("APP_URL"), "/")
}

// Handler renders the developer portal landing.
func Handler(w http.ResponseWriter, r *http.Request) {
	brand := htmlEscape(app.PortalBrand(r))
	base := canonical() // "" => same-origin

	body := `<p style="max-width:560px;text-align:center;color:#555;font-size:16px;line-height:1.6;margin:0 auto 28px">
Every capability, as a service your agents can call — news, markets, weather, web search,
video, mail and more. Reachable over <a href="/mcp" style="color:#111">MCP</a> and REST, paid
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
		Title:       brand + " — APIs for agents",
		Description: "Every Mu capability as an MCP tool and REST API, paid per call over x402.",
		Brand:       brand,
		Tagline:     "APIs for agents",
		TopRight:    `<a href="` + base + `/login">Sign in →</a>`,
		Body:        body,
		Image:       "/.portal/logo.svg",
		Footer: `<a href="/mcp">MCP</a>
  <a href="/api">API</a>
  <a href="https://x402.org">x402</a>
  <a href="https://go-micro.dev">Go Micro</a>
  <a href="` + base + `/login">Sign in</a>`,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}

// LogoHandler serves a square wordmark derived from the domain, used as the
// portal's favicon and link-preview image so a shared link shows e.g. "M3O"
// rather than the Mu logo. Domain-agnostic — the text comes from the Host.
func LogoHandler(w http.ResponseWriter, r *http.Request) {
	brand := htmlEscape(app.PortalBrand(r))
	n := len([]rune(app.PortalBrand(r)))
	size := 200
	if n > 3 {
		size = 200 * 3 / n
		if size < 64 {
			size = 64
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="600" viewBox="0 0 600 600">
<rect width="600" height="600" fill="#111"/>
<text x="300" y="300" fill="#fff" font-family="Nunito Sans,-apple-system,Segoe UI,Roboto,sans-serif" font-weight="800" font-size="` + strconv.Itoa(size) + `" text-anchor="middle" dominant-baseline="central">` + brand + `</text>
</svg>`))
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
