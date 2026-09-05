// Package origin answers one question: what is this instance's public address?
package origin

import (
	"net"
	"net/http"
	"strings"

	"mu/internal/settings"
)

// URL returns the public origin for anything a caller outside this process will
// read back: an OAuth issuer, an x402 resource identifier, a payment return URL,
// a link in an email.
//
// A Mu instance may have an optional second public hostname dedicated to its
// public pay-per-call machine interface. If a request arrived through X402_HOST,
// reflect that host back in discovery and payment URLs. This is an additional
// door to the same MCP/API tools, not where the tools themselves live.
func URL(r *http.Request) string {
	if h := requestHost(r); h != "" && sameHost(h, settings.Get("X402_HOST")) {
		return scheme(r) + "://" + trimScheme(h)
	}
	if u := Self(); u != "" {
		return u
	}
	if h := requestHost(r); h != "" {
		return scheme(r) + "://" + trimScheme(h)
	}
	return scheme(r) + "://" + r.Host
}

// IsX402Host reports whether this request arrived on the optional x402 host.
// The host is a canonical public machine/payment door; /tools, /mcp and the API
// remain available on the primary Mu host as well.
func IsX402Host(r *http.Request) bool {
	return sameHost(requestHost(r), settings.Get("X402_HOST"))
}

func requestHost(r *http.Request) string {
	if h := forwardedHost(r); h != "" {
		return h
	}
	return strings.TrimSpace(r.Host)
}

func forwardedHost(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if i := strings.Index(h, ","); i > 0 {
		h = strings.TrimSpace(h[:i])
	}
	return h
}

func sameHost(a, b string) bool {
	a = hostname(a)
	b = hostname(b)
	return a != "" && b != "" && strings.EqualFold(a, b)
}

func hostname(v string) string {
	v = strings.TrimSpace(trimScheme(v))
	v = strings.TrimSuffix(v, "/")
	if h, _, err := net.SplitHostPort(v); err == nil {
		return h
	}
	return v
}

// Self is the public origin when there is no request to derive it from.
func Self() string {
	for _, key := range []string{"MU_DOMAIN", "PUBLIC_URL", "APP_URL"} {
		v := strings.TrimSpace(settings.Get(key))
		if v == "" || v == "localhost" {
			continue
		}
		return "https://" + strings.TrimSuffix(trimScheme(v), "/")
	}
	return ""
}

func scheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

func trimScheme(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
}
