// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/account"
	"mu/agent"
	"mu/agent/brief"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/blog"
	"mu/service/events"
	"mu/service/weather"
	"mu/work"
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
	snapshot, pending := overview(acc)
	content := overviewHTML(r, acc, snapshot)
	// The first visit can fill its cards after paint without replacing the prompt
	// or draft. Subsequent visits render the cached overview immediately.
	if r.URL.Query().Get("view") == "overview" {
		app.RespondJSON(w, map[string]any{"html": content, "pending": pending, "weather": weatherLine(acc.ID)})
		return
	}
	body := `<div data-home-overview>` + `<div class="home-date"><time datetime="` + account.LocalNow(acc.ID).Format("2006-01-02") + `">` + account.LocalNow(acc.ID).Format("Monday, 2 January") + `</time>` + `<span id="home-weather">` + weatherLine(acc.ID) + `</span></div>` + `</div>` + agent.Prompt(acc.ID) + `<div data-home-overview id="home-overview-content" data-pending="` + fmt.Sprint(pending) + `">` + content + `</div>`
	app.Respond(w, r, app.Response{Title: "Home", HTML: body})
}

func tabs(apps bool) string {
	overview, currentApps := ` aria-current="page"`, ""
	if apps {
		overview, currentApps = "", ` aria-current="page"`
	}
	return `<nav class="view-switch form-actions" aria-label="Home"><a href="/home"` + overview + `>Overview</a><a href="/home/apps"` + currentApps + `>My apps</a></nav>`
}

func weatherLine(owner string) string {
	lat, lon, located := auth.Located(owner)
	if located {
		if temperature, description, ok := weather.Now(lat, lon); ok {
			return `<a href="/weather">` + html.EscapeString(fmt.Sprintf("%s · %d°C, %s", auth.PlaceName(owner), temperature, description)) + `</a>`
		}
		return `<a href="/weather">Weather in ` + html.EscapeString(auth.PlaceName(owner)) + `</a>`
	}
	return `<a href="/account#place">Set your location for weather</a>`
}

// Brief contains only the cached world summary; rendering never calls a model.
func shortBrief() string {
	line := brief.Line()
	if line == "" {
		return ""
	}
	return `<section class="section-card" aria-labelledby="home-brief-title"><div class="section-card-head"><h2 id="home-brief-title">Brief</h2></div><p class="home-summary">` + html.EscapeString(line) + `</p></section>`
}

func overviewHTML(r *http.Request, acc *auth.Account, snapshot overviewSnapshot) string {
	var left, right strings.Builder
	if preview := inbox.Preview(acc.ID); preview != "" {
		left.WriteString(app.PreviewCard("home-inbox", "Inbox", "/inbox", preview))
	}
	reading := blog.Preview()
	if reading == "" {
		reading = `<p class="text-muted">Published articles and topic digests will appear here.</p>`
	}
	left.WriteString(app.PreviewCard("home-blog", "Blog", "/blog", `<div class="home-card-content">`+reading+`</div>`))
	right.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	right.WriteString(work.ScheduledCard(acc.ID))
	extra := 0
	for _, spec := range overviewServices(acc) {
		column := &left
		switch spec.Name {
		case "markets", "video":
			column = &right
		case "news":
			// Reading follows Inbox and Blog in the left column.
		default:
			if extra%2 != 0 {
				column = &right
			}
			extra++
		}
		body := snapshot.cards[spec.Name]
		if body == "" {
			body = `<p class="text-muted">` + html.EscapeString(spec.Description) + `</p>`
		}
		column.WriteString(app.PreviewCard("home-service-"+spec.Name, spec.NavLabel(), spec.Page, `<div class="home-card-content">`+body+`</div>`))
	}
	columns := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		columns = `<div class="page-col">` + right.String() + `</div>`
	}
	return `<div class="page-col">` + todoHTML(acc.ID) + shortBrief() + `<nav class="form-actions" aria-label="Home services"><a href="/services">Pin services to Home</a></nav>` + columns + `</div>`
}
