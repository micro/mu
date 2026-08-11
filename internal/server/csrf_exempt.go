package server

// Who does not need a CSRF token.
//
// The token proves a state-changing request came from a page this instance
// served, which is the right question to ask a browser and a meaningless one to
// ask anything else. Everything below is something else, and every one of them
// authenticates some other way.
//
// This was a run of booleans inline in the middleware, and the cost of that was
// a real outage: the webhook exemption named the Stripe path exactly, so
// Twilio's inbound messages were refused with a 403 before reaching a handler
// that had a signature check waiting for them. Twilio reported it as 11200,
// "HTTP retrieval failure", which is what it looks like from outside — and
// nothing on this side logged anything, because the request never got far
// enough to be anybody's.

import (
	"net/http"
	"strings"
)

// csrfExempt reports whether this request is one the CSRF token cannot speak
// for.
func csrfExempt(r *http.Request) bool {
	switch {
	// A bearer token or a PAT is the credential, and neither is a cookie, so
	// there is no cross-site request to forge.
	case r.Header.Get("Authorization") != "", r.Header.Get("X-Micro-Token") != "":
		return true

	// MCP carries its own auth.
	case r.URL.Path == "/mcp":
		return true

	// A provider posting to a webhook. Stripe, Twilio, Meta — none of them has
	// a session here, and each is authenticated by a signature over the body
	// that its own handler checks. By suffix rather than by name, because the
	// alternative is remembering to add each one and finding out you did not
	// when a provider says it could not reach you.
	case strings.HasSuffix(r.URL.Path, "/webhook"):
		return true

	// Signing in, where there is no session to have a token from yet.
	case r.URL.Path == "/login", r.URL.Path == "/signup", r.URL.Path == "/request-invite":
		return true
	case strings.HasPrefix(r.URL.Path, "/passkey/"), strings.HasPrefix(r.URL.Path, "/oauth/"):
		return true

	// SMTP and ActivityPub delivery.
	case strings.HasSuffix(r.URL.Path, "/inbox"):
		return true
	}
	return false
}
