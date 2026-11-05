package auth

import (
	"html"
	"net/http"
	"strings"
)

// A form redirect into a custom scheme can be swallowed by mobile browsers.
// Keep approval on this origin and give the user an explicit app-launch gesture.
func finishAuthorization(w http.ResponseWriter, r *http.Request, redirectURI, code, state string) {
	callback := authorizationRedirect(redirectURI, code, state)
	if !nativeRedirect(redirectURI) {
		http.Redirect(w, r, callback, http.StatusSeeOther)
		return
	}
	// Pin the intent to the matching installed app, rather than asking any app
	// that claimed the URI scheme to handle the authorization code.
	scheme := strings.SplitN(redirectURI, ":", 2)[0]
	intent := "intent:" + strings.TrimPrefix(callback, scheme+":") +
		"#Intent;scheme=" + scheme + ";package=" + scheme + ";end"
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Return to Micro</title><link rel="stylesheet" href="/mu.css"></head><body class="oauth-consent"><main><h1>Access approved</h1><p>Open Micro to finish signing in.</p><div class="form-actions"><a class="btn" href="` + html.EscapeString(intent) + `">Open Micro</a></div><p class="text-muted">If the app does not open, check that Micro is installed on this device. Start sign-in again from the app if this page has expired.</p></main></body></html>`))
}
