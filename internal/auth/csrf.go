package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
)

var (
	csrfKey     []byte
	csrfKeyOnce sync.Once
)

// csrfSecret returns a per-process secret key for HMAC-based CSRF tokens.
// Generated once at startup, lost on restart (which invalidates all tokens —
// users just need to reload the page).
func csrfSecret() []byte {
	csrfKeyOnce.Do(func() {
		csrfKey = make([]byte, 32)
		rand.Read(csrfKey)
	})
	return csrfKey
}

// CSRFToken returns a CSRF token derived from the session ID.
// Returns empty string if there is no session.
func CSRFToken(r *http.Request) string {
	sess, _ := TrySession(r)
	if sess == nil {
		return ""
	}
	return csrfTokenFor(sess.ID)
}

func csrfTokenFor(sessionID string) string {
	mac := hmac.New(sha256.New, csrfSecret())
	mac.Write([]byte(sessionID))
	return hex.EncodeToString(mac.Sum(nil))
}

// SetCSRFCookie sets a non-httpOnly cookie with the CSRF token so JavaScript
// can read it and include it in fetch requests via the X-CSRF-Token header.
func SetCSRFCookie(w http.ResponseWriter, r *http.Request) {
	token := CSRFToken(r)
	if token == "" {
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		MaxAge:   2592000, // 30 days, same as session
		HttpOnly: false,   // JS needs to read this
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ValidCSRF checks that a state-changing request carries a valid CSRF token.
// The token can be in:
//   - Header "X-CSRF-Token" (JS fetch calls)
//   - Form field "_csrf" (HTML form submissions)
//
// Returns true if:
//   - The request has no session (unauthenticated)
//   - A valid token is provided
//   - No token is provided at all (grace period for clients with stale JS)
//
// Returns false only if a token IS provided but is invalid.
func ValidCSRF(r *http.Request) bool {
	sess, _ := TrySession(r)
	if sess == nil {
		return true
	}

	expected := csrfTokenFor(sess.ID)

	// Check header first (JS fetch calls)
	if token := r.Header.Get("X-CSRF-Token"); token != "" {
		return hmac.Equal([]byte(token), []byte(expected))
	}

	// Check form field (HTML forms)
	if token := r.FormValue("_csrf"); token != "" {
		return hmac.Equal([]byte(token), []byte(expected))
	}

	// No token provided — allow for now (stale JS / cached pages).
	// The SetCSRFCookie middleware ensures the cookie is set for next time.
	return true
}

// StrictCSRF is ValidCSRF without the grace period: a request carrying a
// session and no token is refused.
//
// The grace above means CSRF is not, today, enforced anywhere — a request that
// simply omits the token is allowed, which is what a forging page would do.
// That is a deliberate compatibility decision for pages and scripts this
// instance has already served, and it is not this function's job to overturn
// it; changing ValidCSRF would break every form that has not been reloaded.
//
// A door opened today has no stale clients, so it can start correct. The API
// door uses this for state-changing calls that rest on a cookie. Anything
// authenticating by header — a token, a wallet, a payment — is not forgeable
// cross-site in the first place and never reaches here.
func StrictCSRF(r *http.Request) bool {
	sess, _ := TrySession(r)
	if sess == nil {
		return true
	}
	expected := csrfTokenFor(sess.ID)
	if token := r.Header.Get("X-CSRF-Token"); token != "" {
		return hmac.Equal([]byte(token), []byte(expected))
	}
	if token := r.FormValue("_csrf"); token != "" {
		return hmac.Equal([]byte(token), []byte(expected))
	}
	return false
}
