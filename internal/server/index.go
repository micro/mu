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

// Explain everyday help and optional scheduled events after the main introduction.
func landingHTML() string {
	return `<section class="landing-introduction">
<h1>Micro</h1>
<p class="landing-tagline">A personal assistant for your everyday life.</p>
<p>Ask a question, explore an idea or get stuff done.</p>
<ul class="landing-highlights" aria-label="Ways to use Micro">
<li><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 4h16v12H9l-5 4V4ZM8 8h8M8 12h5"/></svg><span>Ask</span></li>
<li><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5 19 19M5 19l1.5-1.5M17.5 6.5 19 5"/></svg><span>Brief</span></li>
<li><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 4h16v12H9l-5 4V4Zm4 6 3 3 5-5"/></svg><span>Chat</span></li>
<li><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v16H4V5ZM8 3v4M16 3v4M4 10h16M8 14h3M8 17h7"/></svg><span>Plan</span></li>
<li><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 6V3h8v3M3 7h18v14H3V7ZM3 12h18M10 12v3h4v-3"/></svg><span>Work</span></li>
</ul>
<div class="form-actions"><a class="btn landing-primary" href="/pricing">Get started</a></div>
</section>
<section class="landing-introduction landing-scheduled">
<h2>What Micro can do</h2>
<p>Start a conversation whenever you need a hand.</p>
<dl class="landing-capabilities">
<div><dt>Find answers</dt><dd>Search the web, explore a topic or catch up on the latest news and local information.</dd></div>
<div><dt>Get stuff done</dt><dd>Work with your notes, files and tasks. Connect Google Calendar to include your schedule.</dd></div>
<div><dt>Talk your way</dt><dd>Use Micro on the web, by email, WhatsApp or XMPP.</dd></div>
</dl>
<p class="landing-access">Tell Micro what you need in your own words. It can look things up, use the information you share and help you take the next step. Ask follow-up questions as you go.</p>
</section>
<section class="landing-introduction landing-scheduled">
<h2>Scheduled Events</h2>
<p>Opt into scheduled events and choose when they reach you.</p>
<dl class="landing-capabilities">
<div><dt>Morning Brief</dt><dd>Your day, weather, prayer times, headlines and a daily reminder in a familiar format.</dd></div>
<div><dt>Daily Check-in</dt><dd>A short prompt to share what’s on your mind and what you need to get done. Reply in a sentence or two.</dd></div>
<div><dt>Evening Research</dt><dd>Choose a topic and schedule. Micro searches the web and sends a summary with source links when it finds an update.</dd></div>
</dl>
<p class="landing-access">Free includes a weekly brief; Starter and Pro include daily briefs. Research is available with Pro and uses credits. You choose which events to enable.</p>
</section>`
}
