package server

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
)

// X402PageHandler explains wallet-paid access and points to the configured host.
func X402PageHandler(w http.ResponseWriter, r *http.Request) {
	body := `<div class="section-stack"><section><h2>Pay per call</h2><p>Use public services from your own agent with x402 payments. You do not need a Micro account for public paid tools. Private account data still requires authorization.</p></section><section><h2>How it works</h2><ol><li>Choose a tool from the catalogue and call it through MCP or the HTTP API.</li><li>A paid call returns HTTP 402 with payment requirements.</li><li>Your x402-compatible client signs the payment and retries the request.</li></ol><p>Free public tools do not require payment. An ordinary MCP client needs x402 payment support to use this flow.</p></section>`
	host := strings.TrimSpace(settings.Get("X402_HOST"))
	if host != "" {
		if !strings.Contains(host, "://") {
			host = "https://" + host
		}
		if u, err := url.Parse(host); err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") {
			base := html.EscapeString(strings.TrimRight(u.Scheme+"://"+u.Host, "/"))
			body += `<section><h2>Connect</h2><p><a href="` + base + `">Open the x402 host</a></p><p>MCP endpoint: <code>` + base + `/mcp</code></p><p>HTTP API: <code>` + base + `/api/v1/</code></p><p><a href="` + base + `/tools">Host catalogue</a></p></section>`
		}
	} else {
		body += `<p>A separate x402 host is not configured on this instance.</p>`
	}
	body += `<div class="form-actions"><a href="/tools">Tools and prices</a><a href="/api">API reference</a><a href="/developers">Use your Micro account</a></div></div>`
	app.Respond(w, r, app.Response{Title: "x402", Description: "Service access with wallet payments", HTML: body})
}
