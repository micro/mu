package server

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/auth"
)

// Browser cookies are ambient credentials. An Authorization header or an MCP
// path does not exempt a request that actually authenticated with a cookie.
// Older first-party pages without a CSRF token may prove their origin instead.
func browserWriteAllowed(r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return true
	}
	cookie, err := r.Cookie("session")
	if err != nil {
		return true
	}
	if _, err := auth.ParseToken(cookie.Value); err != nil {
		return true
	}
	if auth.StrictCSRF(r) {
		return true
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
