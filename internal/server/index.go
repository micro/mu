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
<p>Ask a question, work through an idea or get help with something you need to do.</p>
<p>Talk to Micro here on the web, by email, WhatsApp or XMPP. Your assistant is there when you need it.</p>
<p class="landing-access">Search the web, find local information and work with your notes, files and tasks. Connect Google Calendar to bring your schedule into the conversation.</p>
<div class="form-actions"><a class="btn landing-primary" href="/pricing">Get started</a></div>
<a class="landing-services" href="/services">Explore services</a>
</section>
<section class="landing-introduction landing-scheduled">
<h2>Help on your schedule</h2>
<p>Opt into scheduled events and choose when they reach you.</p>
<dl class="landing-capabilities">
<div><dt>Morning brief</dt><dd>Your day, weather, prayer times, headlines and a daily reminder in a familiar format.</dd></div>
<div><dt>Daily check-in</dt><dd>A short prompt to share what’s on your mind and what you need to get done. Reply in a sentence or two.</dd></div>
<div><dt>Evening research</dt><dd>Follow a topic with source-linked updates to read when you have time to think. Choose your time and frequency.</dd></div>
</dl>
<p class="landing-access">Free includes a weekly brief; Starter and Pro include daily briefs. Research is available with Pro and uses credits. You choose which events to enable.</p>
</section>`
}
