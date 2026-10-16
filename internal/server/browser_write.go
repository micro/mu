package server

import (
	"net/http"

	"mu/internal/auth"
	"mu/internal/origin"
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
	if r.Header.Get("Origin") != "" {
		return origin.SameOrigin(r)
	}

	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
