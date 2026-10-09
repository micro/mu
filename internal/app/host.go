package app

import (
	"html"
	"mu/internal/origin"
	"net/http"
	"net/url"
	"strings"
)

// hostHTML is the public developer shell over the same catalogue and assets.
func hostHTML(title, description, body string, r *http.Request) string {
	u, _ := url.Parse(origin.URL(r))
	parts := strings.Split(u.Hostname(), ".")
	name := strings.ToUpper(parts[0])
	if len(parts) > 1 {
		name = strings.ToUpper(parts[len(parts)-2])
	}
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/tools/") {
		body = `<h1>` + html.EscapeString(title) + `</h1><p class="text-muted">` + html.EscapeString(description) + `</p>` + body
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="theme-color" content="#111315"><meta name="description" content="` + html.EscapeString(description) + `"><title>` + html.EscapeString(title+" | "+name) + `</title><link rel="stylesheet" href="/mu.css?v=x402-1"><script defer src="/mu.js?v=x402-1"></script></head><body class="x402-site"><div class="page"><header><a class="brand" href="/">` + html.EscapeString(name) + `</a><nav aria-label="Navigation"><a href="/tools">Tools</a><a href="/pricing">Pricing</a><a href="/api">API</a></nav></header><main>` + body + `</main></div><footer><a href="/mcp">MCP</a><a href="/llms.txt">Agent discovery</a><a href="https://github.com/micro/mu">Powered by Mu</a></footer></body></html>`
}
