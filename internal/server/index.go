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

// The public page demonstrates Micro without creating a personal account.
func landingHTML() string {
	return `<section class="landing-introduction">
<h1>Micro</h1>
<p class="landing-tagline">An open personal assistant.</p>

<div class="public-demo"><form data-demo-form method="post" action="/"><label class="sr-only" for="demo-input">Ask Micro</label><div class="composer"><textarea id="demo-input" name="prompt" rows="1" maxlength="1000" placeholder="Ask Micro a question" required></textarea><button type="submit">Send</button></div></form><div data-demo-responses role="log" aria-label="Conversation"></div><p data-demo-status role="status"></p></div>
</section>`
}
