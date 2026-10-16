package server

import (
	"fmt"
	"html"
	"mu/agent"
	"mu/agent/brief"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/x402/billing"
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
	price := "Personal chat and tools use prepaid credits: 1 credit = 1 US cent."
	if p, ok := billing.SubscriptionPlan("starter"); ok {
		price = "From " + billing.Money(p.Cents) + "/month, or use prepaid credits."
	}
	if !billing.PaymentsEnabled() {
		price = "Self-hosted on this instance."
	}
	summary := `<p>Today’s brief is being prepared. <a href="/news">Read the headlines</a> or <a href="/blog">browse the digests</a>.</p>`
	if entry, ok := brief.Latest(); ok {
		summary = `<p>` + html.EscapeString(entry.Text) + `</p><p><a href="/brief?id=` + entry.ID() + `">Sources and details</a> · <a href="/blog">Read the digests</a></p>`
	}
	return `<section class="landing-introduction">
<h1>Micro</h1><p class="landing-tagline">An open personal assistant.</p>
<p>Ask a question. Get a brief. Put recurring work in Micro’s hands.</p>
<div class="public-demo"><form data-demo-form method="post" action="/"><label class="sr-only" for="demo-input">Ask Micro</label><div class="composer"><textarea id="demo-input" name="prompt" rows="2" maxlength="1000" placeholder="Ask Micro a question" required></textarea><button type="submit">Send</button></div><p class="text-muted">Try a short chat. Personal tools and saved conversations require payment.</p></form><div data-demo-responses role="log" aria-label="Conversation"></div><p data-demo-status role="status"></p></div>
<p>` + html.EscapeString(price) + `</p><div class="form-actions"><a class="btn landing-primary" href="/pricing">Activate your assistant</a><a href="/login">Log in</a></div>
</section><section class="landing-introduction landing-scheduled"><h2>Today’s brief</h2>` + summary + `</section>`
}
