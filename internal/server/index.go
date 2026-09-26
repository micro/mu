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

// Lead with what the assistant does; the service catalogue remains a secondary door.
func landingHTML() string {
	return `<div class="landing-introduction">
<section class="landing-hero">
<p class="landing-eyebrow">A little help with everyday life</p>
<h1>Less to keep up with.<br>More time for you.</h1>
<p class="landing-lead">Micro helps you catch up, make a plan and follow through. An assistant for the things you need to know, remember and get done.</p>
<div class="form-actions"><a class="btn landing-primary" href="/signup">Get started</a><a href="/pricing">Find your plan</a></div>
<p class="landing-note">Start free. No card needed.</p>
</section>
<section class="landing-outcomes" aria-label="How Micro helps">
<article><span class="landing-label">Brief</span><h2>Start with a clearer day.</h2><p>Your upcoming plans, outstanding work and recent conversations, brought together in a short brief. A weekly catch-up on Free, or a morning brief with a subscription.</p></article>
<article><span class="landing-label">Plan</span><h2>Know what to do next.</h2><p>Ask Micro to turn a busy day into a manageable plan. With Pro, add suggested priorities to your morning brief, using the commitments and work you already have.</p></article>
<article><span class="landing-label">Research</span><h2>Keep up without the digging.</h2><p>Get help finding answers. With Pro, choose a topic to follow and receive updates when there’s something new, with sources and a spending limit you set.</p></article>
</section>
<section class="landing-followthrough"><div><p class="landing-eyebrow">Less remembering. More getting things done.</p><h2>Give the repeat work to Micro.</h2><p>Set a standing instruction once. Micro can run it on a schedule and send the result to your inbox, so you don’t have to start the same conversation every time.</p><p>Keep the details that matter, pick up where you left off, and work through your to-do list with help along the way.</p></div><aside class="landing-example" aria-label="Example request"><span class="landing-label">Something you could ask</span><p>“Every Friday, summarise my outstanding work and suggest what to focus on next week.”</p><span class="landing-note">Scheduled work uses credits. You choose what runs.</span></aside></section>
<section class="landing-close"><h2>A little more room in your day.</h2><p>Start a conversation. Let Micro help with the next thing.</p><div class="form-actions"><a class="btn landing-primary" href="/signup">Get started</a><a href="/services">Explore the tools behind Micro</a></div></section>
</div>`
}
