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

// Explain the everyday outcomes and available context before sending visitors
// to Pricing or the full service directory.
func landingHTML() string {
	return `<section class="landing-introduction">
<h1>Micro</h1>
<p class="landing-tagline">A personal assistant for your everyday life.</p>
<p>Spend less time looking things up and keeping track of everything.</p>
<dl class="landing-capabilities">
<div><dt>Morning Brief</dt><dd>Catch up on your schedule, outstanding tasks and the news in one brief.</dd></div>
<div><dt>Daily Checkin</dt><dd>Say what you need to get done in one or two sentences. Micro helps you work out the next steps.</dd></div>
<div><dt>Deep Research</dt><dd>Look things up, compare options and save useful findings, with links to sources.</dd></div>
</dl>
<p class="landing-access">Web search, news, weather and maps are built in. Micro works with the notes, files, mail and tasks you keep here. Connect Google Calendar to include your calendar too.</p>
<div class="form-actions"><a class="btn landing-primary" href="/pricing">Get started</a></div>
<a class="landing-services" href="/services">Explore services</a>
</section>`
}
