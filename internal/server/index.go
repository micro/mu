package server

import (
	"fmt"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// IndexHandler owns the public front door. Legacy conversation URLs remain
// usable, but signed-in visits without a conversation go to Home.
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if mucpRootRequest(r) {
		MUCPHandler(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodPost {
		if _, acc := auth.TrySession(r); acc == nil {
			agent.DemoHandler(w, r)
			return
		}
	}
	auth.SetCSRFCookie(w, r)
	if r.Method == http.MethodPost || r.URL.Query().Get("session") != "" || r.URL.Query().Get("continue") != "" || r.URL.Query().Get("agent") != "" {
		agent.ConsoleHandler(w, r)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if _, acc := auth.TrySession(r); acc != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, app.ConsoleHTML("Micro", landingHTML(), nil))
}

// The public front door separates using the assistant from running the runtime.
func landingHTML() string {
	return `<section class="landing-introduction">
<h1>Micro</h1>
<p class="landing-tagline">A personal assistant</p>
<p>Talk, get things done, and make room for what matters.</p>
<div class="form-actions" data-client-downloads><a class="btn" href="/app/" data-open-assistant>Open assistant</a><a class="btn" href="/download#android" data-android-download>Android</a></div>
<p class="landing-access"><a href="/download">Other platforms and installation</a></p>
<p class="landing-access"><a href="/install">Self-host Mu</a> · <a href="/developers">Developer tools</a></p>
</section>`
}
