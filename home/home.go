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
	return app.PreviewSection("home-brief", "Brief", "", `<p class="home-summary">`+html.EscapeString(line)+`</p>`)
}

func overviewHTML(r *http.Request, acc *auth.Account, snapshot overviewSnapshot) string {
	var left, right strings.Builder
	left.WriteString(todoHTML(acc.ID))
	if preview := inbox.Preview(acc.ID); preview != "" {
		left.WriteString(app.PreviewSection("home-inbox", "Messages", "/inbox", preview))
	}
	right.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	right.WriteString(work.ScheduledCard(acc.ID))
	today := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		today = `<div class="page-stack">` + right.String() + `</div>`
	}
	left.Reset()
	right.Reset()
	if reading := blog.Preview(); reading != "" {
		left.WriteString(app.PreviewSection("home-blog", "Reading", "/blog", `<div class="home-card-content">`+reading+`</div>`))
	}
	extra := 0
	for _, spec := range overviewServices(acc) {
		column := &left
		switch spec.Name {
		case "markets", "video":
			column = &right
		case "news":
			// News follows articles in the reading column.
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
		column.WriteString(app.PreviewSection("home-service-"+spec.Name, spec.NavLabel(), spec.Page, `<div class="home-card-content">`+body+`</div>`))
	}
	columns := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		columns = `<div class="page-col">` + right.String() + `</div>`
	}
	return `<div class="page-stack home-overview">` + today + shortBrief() + columns + `<nav class="form-actions" aria-label="Home services"><a href="/services">Choose services</a></nav></div>`
}
