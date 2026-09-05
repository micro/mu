// Package origin answers one question: what is this instance's public address?
//
// It exists because three places answered it differently and two were wrong.
// The x402 challenge advertised https://localhost:8081/mcp as the resource
// being paid for. The OAuth discovery documents — the ones an MCP client reads
// to find out how to authenticate — advertised https://localhost:8081 as the
// authorization server, which points every client at a host it cannot reach.
// The Stripe return URL was believed to have the logic right, inline, where
// nothing else could use it. It did not: it guessed the scheme from whether
// X-Forwarded-Proto was present and took the host from r.Host, so paying for a
// subscription returned the customer to https://localhost:8081/account. That is
// the fourth answer, and it was the one held up as correct — which is the
// argument for this package rather than against it. Inline logic nobody can
// call is inline logic nobody can check.
//
// It lives in its own package because the callers cannot share one otherwise:
// internal/app imports internal/auth, so auth cannot import app.
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
// A Mu instance may have an optional second public hostname for x402. The
// host is configuration, not a second deployment: MCP, API and x402 still run
// in this process. If the request arrived through X402_HOST, that host is the
// public identity and must be reflected back in discovery/payment URLs.
//
// We only trust a forwarded/request host when it matches the configured x402
// host. That lets a normal reverse proxy preserve Host/X-Forwarded-Host without
// making arbitrary client-supplied forwarded headers authoritative.
func URL(r *http.Request) string {
	// Preserve the explicit surface selected by existing reverse proxies. The
	// header is an opt-in trust boundary; X-Forwarded-Host alone remains
	// insufficient to override the configured instance origin.
	if strings.TrimSpace(r.Header.Get("X-Mu-Surface")) != "" {
		if h := forwardedHost(r); h != "" {
			return scheme(r) + "://" + trimScheme(h)
		}
	}
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

// IsX402Host reports whether this request arrived on the configured optional
// x402 hostname. It is intentionally about a hostname, not an audience or a
// product brand: operators are free to name and use that second door however
// they like.
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
//
// Background work has the same question and none of the answers: a scheduled
// job, a service naming this instance to another one, anything that runs
// without somebody having asked. It was reachable only through URL(r), so those
// callers each invented their own — and the wallet's "self" server read an
// APP_URL that nothing sets, which made paying a tool on this instance fail
// with "no server called".
//
// Returns "" when nothing is configured, which is the honest answer: a caller
// with no request and no MU_DOMAIN genuinely cannot know its own address, and
// guessing localhost is how a URL nobody can reach gets published.
func Self() string {
	for _, key := range []string{"MU_DOMAIN", "PUBLIC_URL", "APP_URL"} {
		v := strings.TrimSpace(settings.Get(key))
		if v == "" || v == "localhost" {
			continue
		}
		// Always https, whatever was configured. This is the address handed to
		// somebody else — an OAuth issuer, an x402 resource, a link in a mail —
		// and http there is a downgrade published in writing.
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
