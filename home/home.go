// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"html"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.URL.Query().Get("session") != "" || r.URL.Query().Get("continue") != "" {
		agent.ConsoleHandler(w, r)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<div data-home-overview>` + tabs(false) + `</div>` + agent.Prompt(acc.ID) + `<div data-home-overview>` + deliveredBrief(acc.ID) + `</div>`
	app.Respond(w, r, app.Response{Title: "Home", HTML: body})
}

func tabs(apps bool) string {
	overview, currentApps := ` aria-current="page"`, ""
	if apps {
		overview, currentApps = "", ` aria-current="page"`
	}
	return `<nav class="view-switch form-actions" aria-label="Home"><a href="/home"` + overview + `>Overview</a><a href="/home/apps"` + currentApps + `>My apps</a></nav>`
}

func deliveredBrief(owner string) string {
	body, id, at := latestBrief(owner)
	if id == "" {
		return `<section class="record-card"><h2>Daily brief</h2><p>Your latest delivered brief will appear here.</p><a href="/events?view=brief">Manage daily brief</a></section>`
	}
	return `<section class="record-card"><h2>Daily brief</h2>` + briefDeliveredAt(at) + `<div class="markdown-content">` + app.RenderString(body) + `</div><div class="form-actions"><a href="/inbox?id=` + url.QueryEscape(id) + `">Open conversation</a><a href="/events?view=brief">Manage daily brief</a></div></section>`
}

// Read Inbox's local projection only. Rendering never calls a source service,
// fetches news or runs a model. The original message date survives replies.
func latestBrief(owner string) (string, string, time.Time) {
	m := thread.LatestTo(owner, owner+"+brief")
	if m == nil {
		return "", "", time.Time{}
	}
	return strings.SplitN(m.Text, "\n\n---\n", 2)[0], m.Thread, m.At
}

func briefDeliveredAt(at time.Time) string {
	if at.IsZero() {
		return `<p class="text-muted">Delivery date unavailable</p>`
	}
	return `<p class="text-muted">Delivered <time datetime="` + at.UTC().Format(time.RFC3339) + `">` + html.EscapeString(at.UTC().Format("2 January 2006 at 15:04 UTC")) + `</time></p>`
}
