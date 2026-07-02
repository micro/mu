package app

import (
	"net"
	"net/http"
	"strings"
)

// PortalMode reports whether a request should render the developer/API portal
// face rather than the consumer app. It is triggered generically — never by a
// specific domain — so every self-hosted instance behaves identically:
//
//   - a reverse proxy sets "X-Mu-Portal: developer" for a dedicated domain
//     (this is how a vanity API domain gets the portal at its root), or
//   - the request is under the always-available /developers path.
//
// Nothing about any particular domain is compiled in; mapping a domain to this
// face is purely the operator's proxy config. See docs/DEVELOPER_PORTAL.md.
func PortalMode(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Mu-Portal"), "developer") {
		return true
	}
	return r.URL.Path == "/developers" || strings.HasPrefix(r.URL.Path, "/developers/")
}

// PortalBrand returns the wordmark for the developer portal. It is derived from
// the request Host — the second-level label, uppercased — so whatever domain is
// pointed at this instance brands itself: m3o.com -> "M3O", api.acme.dev ->
// "ACME". A reverse proxy may override the exact text with X-Mu-Portal-Brand
// (for casing, multi-part TLDs like foo.co.uk, or a specific name). The consumer
// product keeps its own identity ("Mu"); only the portal face auto-brands.
func PortalBrand(r *http.Request) string {
	if b := strings.TrimSpace(r.Header.Get("X-Mu-Portal-Brand")); b != "" {
		return b
	}
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i] // strip port
	}
	// No meaningful public host (empty, localhost, a bare IP, or a single-label
	// host) → the product's own name rather than a junk wordmark like "LOCALHOST".
	// Note: a reverse proxy must forward the real Host (proxy_set_header Host
	// $host) for a vanity domain to brand itself; otherwise this falls back to Mu.
	if host == "" || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "Mu"
	}
	for _, p := range []string{"api.", "dev.", "developers.", "www."} {
		host = strings.TrimPrefix(host, p)
	}
	labels := strings.Split(host, ".")
	sld := labels[len(labels)-2] // second-to-last label
	if sld == "" {
		return "Mu"
	}
	return strings.ToUpper(sld)
}
