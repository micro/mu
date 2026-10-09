// Package x402 owns the developer host: browser account flows, discovery and
// routing. Billing, payment settlement and the API gate live below this package.
package x402

import (
	"embed"
	"fmt"
	"net/http"
	"strings"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/service/wallet"
	"mu/x402/billing"
	"mu/x402/gateway"
)

//go:embed style.css
var assets embed.FS

// Handler is the sole entry point for the configured x402 host. Unknown paths
// never fall through into Micro's consumer pages.
func Handler(w http.ResponseWriter, r *http.Request) {
	r = browserSession(r)
	if len(r.URL.Path) > 1 {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
	}
	r = app.WithRenderer(r, renderHTML)
	w.Header().Set("Cache-Control", "private, no-store")
	if api.ToolDispatch(r.URL.Path) {
		gateway.Handler(http.HandlerFunc(protocolHandler)).ServeHTTP(w, r)
		return
	}
	// Provider webhooks authenticate their own signatures, without browser cookies.
	if r.URL.Path == "/stripe/webhook" {
		billing.HandleStripeWebhook(w, r)
		return
	}
	switch r.URL.Path {
	case "/oauth/register":
		auth.OAuthRegisterHandler(w, r)
		return
	case "/oauth/token":
		auth.OAuthTokenHandler(w, r)
		return
	case "/.well-known/oauth-authorization-server":
		auth.OAuthMetadataHandler(w, r)
		return
	case "/.well-known/oauth-protected-resource":
		auth.OAuthResourceHandler(w, r)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	}
	auth.SetCSRFCookie(w, r)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !browserOriginAllowed(r) {
			app.Forbidden(w, r, "Reopen this page and try again.")
			return
		}
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Reload this page and try again.")
			return
		}
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case path == "":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			app.MethodNotAllowed(w, r)
			return
		}
		indexHandler(w, r)
	case path == "/mu.css":
		css, _ := assets.ReadFile("style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprint(w, app.Styles()+string(css))
	case path == "/mu.js" || path == "/favicon.ico":
		app.Serve(billing.Script).ServeHTTP(w, r)
	case path == "/login":
		loginHandler(w, r)
	case path == "/logout":
		logoutHandler(w, r)
	case path == "/pricing":
		if r.Method != "GET" && r.Method != "HEAD" {
			app.MethodNotAllowed(w, r)
			return
		}
		pricingHandler(w, r)
	case path == "/tools" || path == "/services":
		toolsHandler(w, r)
	case strings.HasPrefix(path, "/tools/"):
		api.ToolPage(w, r, false)
	case path == "/api":
		api.RESTPageHandler(w, r)
	case path == "/llms.txt":
		discoveryHandler(w, r)
	case path == "/oauth/authorize":
		authorizeHandler(w, r)
	case path == "/account" || path == "/account/billing":
		accountHandler(w, r)
	case path == "/account/tokens":
		tokensHandler(w, r)
	case path == "/account/usage":
		if requireAccount(w, r) != nil {
			billing.UsageHandler(w, r)
		}
	case path == "/account/topup" || path == "/account/transfer" || path == "/stripe/checkout" || path == "/stripe/success":
		if requireAccount(w, r) != nil {
			billing.BalanceHandler(w, r)
		}
	case path == "/wallet":
		if requireAccount(w, r) != nil {
			billing.Wallet(w, r)
		}
	case path == "/wallet/export":
		if requireAccount(w, r) != nil {
			wallet.ExportHandler(w, r)
		}
	case path == "/account/convert":
		if requireAccount(w, r) != nil {
			billing.ConvertUSDC(w, r)
		}
	case path == "/account/crypto":
		if requireAccount(w, r) != nil {
			billing.CryptoHandler(w, r)
		}
	case path == "/account/subscription":
		if requireAccount(w, r) != nil {
			billing.SubscriptionHandler(w, r)
		}
	default:
		app.NotFound(w, r, "Page not found")
	}
}

func protocolHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v1") {
		api.RESTHandler(w, r)
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet && !strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s MCP\n\nEndpoint: %s/mcp\nTransport: streamable-http\nMethods: initialize, tools/list, tools/call\n%s\n", x402Name(), origin.URL(r), hostPaymentDescription())
		return
	}
	api.MCPHandler(w, r)
}
func toolsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		discoveryHandler(w, r)
		return
	}
	app.Respond(w, r, app.Response{Title: "Tools", HTML: `<div class="section-stack">` + connectionHTML(r) + api.ToolsHTML(r, func(t api.Tool) string { return cataloguePrice(t) }) + `</div>`})
}
func discoveryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	base := origin.URL(r)
	fmt.Fprintf(w, "# %s\n\nTools for agents\n\n- MCP: %s/mcp\n- Tools: %s/tools\n- API: %s/api/v1/\n\n%s\n", x402Name(), base, base, base, hostPaymentDescription())
}

// Fetch Metadata is set by the browser and cannot be supplied by page scripts.
// Prefer its same-origin verdict to reconstructing a public URL from proxy
// headers: TLS may terminate upstream without forwarding the external scheme.
// Older clients still have to match the configured public origin when supplied.
func browserOriginAllowed(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin":
		return true
	case "cross-site", "same-site":
		return false
	}
	submitted := r.Header.Get("Origin")
	return submitted == "" || submitted == origin.URL(r)
}
