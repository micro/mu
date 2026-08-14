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
	"net/http"
	"strings"

	"mu/internal/settings"
)

// URL returns the public origin — "https://micro.mu" — for anything a caller
// outside this process will read back: an OAuth issuer, an x402 resource
// identifier, a payment return URL, a link in an email.
//
// r.Host cannot be used on its own. Mu runs behind a reverse proxy that
// forwards to a loopback port, so r.Host is "localhost:8081" and any URL built
// from it names an address no client can reach.
//
// Order: MU_DOMAIN when configured, then the proxy's X-Forwarded-Host, then
// r.Host. X-Forwarded-Host is only trustworthy because the proxy sets it; a
// directly exposed instance would be taking it from the client, which is the
// same assumption ClientIP already makes.
func URL(r *http.Request) string {
	if u := Self(); u != "" {
		return u
	}
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		if i := strings.Index(h, ","); i > 0 {
			h = strings.TrimSpace(h[:i])
		}
		return scheme(r) + "://" + trimScheme(h)
	}
	return scheme(r) + "://" + r.Host
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
