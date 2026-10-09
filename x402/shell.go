package x402

import (
	"crypto/sha256"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// renderHTML is the public developer shell over the same catalogue and assets.
func renderHTML(title, description, body string, r *http.Request) string {
	u, _ := url.Parse(origin.URL(r))
	parts := strings.Split(u.Hostname(), ".")
	name := strings.ToUpper(parts[0])
	if len(parts) > 1 {
		name = strings.ToUpper(parts[len(parts)-2])
	}
	if !strings.Contains(body, "<h1") {
		heading := `<h1>` + html.EscapeString(title) + `</h1>`
		if description != "" {
			heading += `<p class="text-muted">` + html.EscapeString(description) + `</p>`
		}
		body = heading + body
	}
	nav := `<a href="/login">Login</a><a href="/signup">Signup</a>`
	if session, _, err := auth.RequireSession(r); err == nil && session.Type == "account" {
		nav = `<a href="/account">Account</a><a href="/account/tokens">Tokens</a><a href="/account/usage">Usage</a><form method="POST" action="/logout">` + app.CSRFField(auth.CSRFToken(r)) + `<button type="submit" class="nav-link">Signout</button></form>`
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="theme-color" content="#111315"><meta name="description" content="` + html.EscapeString(description) + `"><title>` + html.EscapeString(title+" | "+name) + `</title><link rel="stylesheet" href="/mu.css?v=` + styleVersion() + `"><script defer src="/mu.js?v=x402-1"></script></head><body class="x402-site"><div class="page"><header><a class="brand" href="/">` + html.EscapeString(name) + `</a><nav aria-label="Navigation"><a href="/tools">Tools</a><a href="/pricing">Pricing</a><a href="/api">API</a>` + nav + `</nav></header><main>` + body + `</main></div><footer><a href="/mcp">MCP</a><a href="/llms.txt">Agent discovery</a><a href="https://github.com/micro/mu">Powered by Mu</a></footer></body></html>`
}

var styleOnce sync.Once
var styleRevision string

// Changing either shared or host CSS changes its URL, including for clients
// that cached the older light stylesheet before host routing was installed.
func styleVersion() string {
	styleOnce.Do(func() {
		css, _ := assets.ReadFile("style.css")
		sum := sha256.Sum256([]byte(app.Styles() + string(css)))
		styleRevision = fmt.Sprintf("x402-%x", sum[:8])
	})
	return styleRevision
}
