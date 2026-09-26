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
	w.Header().Set("Cache-Control", "private, no-store")
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

// Keep the public front door compact, with details on Pricing and Services.
func landingHTML() string {
	return `<section class="landing-introduction">
<h1>Micro</h1>
<p class="landing-tagline">A personal assistant for your everyday life.</p>
<p>Catch up on what matters, make a plan and get things done.</p>
<div class="form-actions"><a class="btn landing-primary" href="/signup">Get started</a><a href="/pricing">Pricing</a></div>
<a class="landing-services" href="/services">Explore services</a>
</section>`
}
