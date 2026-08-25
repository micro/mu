// Package hosts answers whether an address is one this instance should go and
// fetch on somebody else's say-so.
//
// The question every service that follows a URL has to ask, and the reason it
// is here rather than in any of them: the answer must be the same everywhere.
// A guard that lives in one service is a guard the next service writes again,
// slightly differently, and the one that gets it wrong is the one nobody was
// looking at.
//
// It was in service/web, where web.Fetch has asked it since that service was
// written. service/browser needs the same answer for the same reason — an
// agent choosing a URL is an agent acting on text a stranger wrote — and a
// service may not import another service, so the shared fact moves down here.
// That is AGENTS.md's own instruction for exactly this case.
//
// # What it is guarding
//
// Server-side request forgery, which on a machine like this is not an abstract
// risk. http://127.0.0.1:8080 is Mu's own admin surface. 169.254.169.254 is the
// cloud metadata endpoint, and on most providers it hands out credentials to
// anything that asks. Both are perfectly ordinary URLs to a fetcher and neither
// is reachable from outside, which is exactly what makes asking this server to
// go and get them worth doing.
package hosts

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Fetchable reports what is wrong with fetching a URL, or nil.
//
// An error rather than a bool, because the caller shows it: "cannot fetch a
// private or internal URL" tells an agent something it can act on, and false
// tells it nothing.
func Fetchable(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid URL: must use http or https")
	}
	if Private(strings.ToLower(u.Hostname())) {
		return fmt.Errorf("cannot fetch a private or internal URL")
	}
	return nil
}

// FetchableString is Fetchable for a URL that has not been parsed yet.
func FetchableString(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	return Fetchable(u)
}

// Private reports whether a host names this machine, this network, or the
// metadata service.
//
// A name that is not an address gets one answer — the well-known metadata
// hostname — and otherwise passes, because resolving every name here would put
// a DNS lookup in front of every fetch and still lose the race: a name that
// resolves to a public address now can resolve to 127.0.0.1 on the second
// lookup the HTTP client makes. What closes that properly is a dialler that
// checks the address it is about to connect to, which is a different mechanism
// in a different place. This is the cheap first gate, not the only one.
func Private(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return PrivateIP(ip)
	}
	return strings.EqualFold(host, "metadata.google.internal")
}

// PrivateIP reports whether an address is one nobody outside this network could
// have reached anyway.
func PrivateIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
