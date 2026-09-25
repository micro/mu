package server

import (
	"fmt"
	"html"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"strings"
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

// The existing service catalogue supplies built-in app destinations and icons.
// Generated apps remain user-owned apps; this introduction does not duplicate them.
func landingHTML() string {
	var b strings.Builder
	b.WriteString(`<section class="app-introduction"><div class="prompt-welcome"><h1>Micro</h1><p>A personal assistant for your everyday life.</p></div><nav class="app-launcher" aria-label="Built-in apps">`)
	for _, s := range service.Pinned([]string{"mail", "events", "notes", "files", "docs", "news", "markets", "video", "weather", "maps"}) {
		b.WriteString(`<a href="` + html.EscapeString(s.Page) + `"><span class="app-launcher-icon"><img src="/` + html.EscapeString(s.NavIcon()) + `" width="32" height="32" alt=""></span><span>` + html.EscapeString(s.NavLabel()) + `</span></a>`)
	}
	b.WriteString(`</nav><p>It has access to your mail, plans, notes and the world around you. Use them directly, or ask Micro to work across them.</p><p><a class="btn" href="/signup">Get started</a></p><p class="text-muted">Ask Micro to plan a visit, add it to your calendar and find the way—all in one conversation.</p><a href="/services">Explore the built-in apps</a></section>`)
	return b.String()
}
