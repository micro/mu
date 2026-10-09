package x402

import (
	"fmt"
	"mu/internal/app"
	"net/http"
	"strings"

	"mu/internal/origin"
	"mu/internal/settings"
	x402 "mu/x402/payment"
)

const x402Description = "Tools for agents"

// IsHost reports whether the request came through the optional x402 host.
// The host is only a second public identity for this same Mu process; it does
// not enable, disable or partition any tools.
func IsHost(r *http.Request) bool { return origin.IsX402Host(r) }

// indexHandler offers a browser landing page and preserves plain-text discovery.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		tagline, connect := "One catalogue. Connect your agent.", "Discover tools and call them with a service token."
		if x402.Enabled() {
			tagline, connect = "One catalogue. Pay per call with x402.", "Discover tools, send a request and pay for priced calls with x402."
		}
		body := `<section class="x402-hero"><h1>Tools for agents</h1><p>Search the web, check the weather, follow markets and more.<br>` + tagline + `</p></section>` + featuredToolsHTML() + `<div class="x402-explore"><a class="btn" href="/tools">Explore tools</a></div><section class="x402-connect"><h2>Ready for your agent</h2><p>Connect through <a href="/mcp">MCP</a> or use the <a href="/api">HTTP API</a>. ` + connect + `</p><p><a href="/pricing">View pricing</a></p></section>`
		app.Respond(w, r, app.Response{Title: "Tools for agents", HTML: body})
		return
	}
	name := x402Name()
	base := strings.TrimRight(origin.URL(r), "/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "%s\n%s\n\n", name, x402Description)
	fmt.Fprintf(w, "MCP: %s/mcp\nTools: %s/tools\nAPI: %s/api/v1/\nLLMs: %s/llms.txt\n", base, base, base, base)
	fmt.Fprintln(w, "\n"+hostPaymentDescription()+" MCP tools/list is the canonical live catalogue.")
}

// x402Name is deliberately derived rather than configured. An x402 hostname
// is the operator's machine-facing identity: m3o.com becomes M3O, foobar.com
// becomes FOOBAR. A normal one-host Mu install has no second identity to name.
func x402Name() string {
	v := strings.TrimSpace(settings.Get("X402_HOST"))
	if v == "" {
		return "Mu"
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	if i := strings.IndexByte(v, '/'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		return strings.ToUpper(parts[len(parts)-2])
	}
	if v != "" {
		return strings.ToUpper(v)
	}
	return "Mu"
}

func hostPaymentDescription() string {
	if x402.Enabled() {
		return "Priced calls use HTTP 402/x402."
	}
	return "Direct x402 payments are not enabled on this instance. Use a service token for account access."
}
