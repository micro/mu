package auth

// Where a code may be sent.
//
// Registration collected redirect_uris and nothing ever read them. /oauth/authorize
// took whatever the query string carried, and — when the visitor already had a
// session — issued a code and redirected there immediately, with no check and no
// consent screen. So a crafted link, clicked once by somebody signed in, delivered
// their authorization code to an address the attacker chose. PKCE does not help:
// in that attack the attacker generates the challenge, so they hold the verifier.
// The exchange compares the redirect_uri from the authorize step with the one from
// the token step, which is consistency between two values the attacker supplied.
//
// Exact string comparison, which is what OAuth 2.1 and RFC 9700 require and what
// the MCP authorization spec restates: no wildcards, no prefixes, no "starts
// with". The one exception is the port of a loopback address, because a native or
// CLI client binds whatever port it can get — RFC 8252 §7.3 — and Mu's own
// registration default is http://localhost:0/callback.

import (
	"errors"
	"net/url"
	"strings"
)

// ErrUnknownClient is an authorize request naming a client nobody registered.
var ErrUnknownClient = errors.New("unknown client_id")

// ErrBadRedirect is a redirect_uri the client did not register.
var ErrBadRedirect = errors.New("redirect_uri does not match one registered by this client")

// RedirectFor returns the address a code may be sent to for this client.
//
// An empty redirect_uri is answered with the client's own, when it registered
// exactly one — a client with a single address does not have to repeat it, and
// the ones registering here overwhelmingly have one.
func RedirectFor(clientID, want string) (string, error) {
	c := GetOAuthClient(clientID)
	if c == nil {
		return "", ErrUnknownClient
	}
	if len(c.RedirectURIs) == 0 {
		return "", ErrBadRedirect
	}
	if want == "" {
		if len(c.RedirectURIs) == 1 {
			return c.RedirectURIs[0], nil
		}
		return "", ErrBadRedirect
	}
	for _, registered := range c.RedirectURIs {
		if sameRedirect(registered, want) {
			return want, nil
		}
	}
	return "", ErrBadRedirect
}

// sameRedirect compares two redirect URIs the way the specification asks:
// byte for byte, except that a loopback address may differ in its port.
func sameRedirect(registered, want string) bool {
	if registered == want {
		return true
	}
	a, err := url.Parse(registered)
	if err != nil {
		return false
	}
	b, err := url.Parse(want)
	if err != nil {
		return false
	}
	if !loopback(a) || !loopback(b) {
		return false
	}
	// Everything but the port, and which spelling of "this machine" was used.
	// A client that registered http://localhost:0/callback and came back on
	// 127.0.0.1:51763 is the same client on the port the operating system gave
	// it; one that came back on a different path is not.
	//
	// Treating the three loopback spellings as one is a deliberate relaxation
	// of exact matching, and a narrow one: both sides have to be loopback
	// already, so what it admits is a caller who could only be reached from the
	// victim's own machine. It is here because registration hands out
	// localhost:0 when a client sends no addresses of its own, and those
	// clients then bind the literal — refusing that would lock out the default
	// this server itself issued.
	return a.Scheme == b.Scheme && a.Path == b.Path && a.RawQuery == b.RawQuery
}

// loopback reports whether a URI addresses this machine. The literals are what
// RFC 8252 names; localhost is included because it is what clients actually
// send and what registration here hands out by default.
func loopback(u *url.URL) bool {
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// RegisterableRedirect reports whether a client may register this address.
//
// Localhost or HTTPS, which is what the MCP authorization spec requires. An
// http:// address anywhere else is a code travelling in clear text, and there
// is no reason to accept one that is not simply a mistake.
func RegisterableRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && loopback(u)
}
