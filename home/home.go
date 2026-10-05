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
	"mu/agent/work"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/blog"
	"mu/service/events"
	"mu/service/weather"
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
	view := "overview"
	if r.URL.Query().Get("view") == "feed" {
		view = "feed"
	}
	snapshot, pending := overview(acc)
	content := overviewHTML(acc)
	source := "/home?view=overview"
	if view == "feed" {
		content = feedHTML(snapshot)
		source = "/home?view=feed"
	}
	// Refresh only the selected view; never replace the prompt or a draft.
	if r.URL.Query().Get("view") == "overview" || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"html": content, "pending": pending, "weather": weatherLine(acc.ID)})
		return
	}
	body := `<div class="page-stack" data-home-overview>` + tabs(view) + `</div>`
	if view == "overview" {
		body += `<div><div data-home-overview class="home-date"><time datetime="` + account.LocalNow(acc.ID).Format("2006-01-02") + `">` + account.LocalNow(acc.ID).Format("Monday, 2 January") + `</time><span id="home-weather">` + weatherLine(acc.ID) + `</span></div>` + agent.Prompt(acc.ID) + `</div>`
	} else {
		body += `<p>News, reading and live updates from your services.</p>`
	}
	body += `<div data-home-overview id="home-overview-content" data-source="` + html.EscapeString(source) + `" data-pending="` + fmt.Sprint(pending) + `">` + content + `</div>`
	app.Respond(w, r, app.Response{Title: "Home", HTML: `<div class="page-stack">` + body + `</div>`})
}

func tabs(active string) string {
	return app.ViewNavigation("Home views", active, []app.ViewLink{
		{Key: "overview", Label: "Overview", URL: "/home"},
		{Key: "feed", Label: "Feed", URL: "/home?view=feed"},
	}, false)
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
	entry, ok := brief.Latest()
	line := entry.Text
	if !ok {
		return ""
	}
	return `<section class="section-card" id="home-brief" aria-labelledby="home-brief-title"><div class="section-card-head"><h2 id="home-brief-title">Brief</h2></div><p class="home-summary">` + html.EscapeString(line) + `</p><a href="/brief?id=` + entry.ID() + `">More</a></section>`
}

func overviewHTML(acc *auth.Account) string {
	var left, right strings.Builder
	left.WriteString(shortBrief())
	if preview := inbox.Preview(acc.ID); preview != "" {
		left.WriteString(app.PreviewCard("home-inbox", "Inbox", "/inbox", preview))
	}
	right.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	right.WriteString(work.ScheduledCard(acc.ID))
	var pins strings.Builder
	for _, spec := range service.Pinned(acc.PinnedServices()) {
		pins.WriteString(`<a class="btn" href="` + html.EscapeString(spec.Page) + `">` + html.EscapeString(spec.NavLabel()) + `</a>`)
	}
	pinned := ""
	if pins.Len() > 0 {
		pinned = `<nav class="form-actions" aria-label="Pinned services">` + pins.String() + `</nav>`
	}
	columns := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		columns = `<div class="page-col">` + right.String() + `</div>`
	}
	return `<div class="page-stack">` + pinned + columns + `</div>`
}

func feedHTML(snapshot overviewSnapshot) string {
	var left, right strings.Builder
	reading := blog.Preview()
	if reading == "" {
		reading = `<p class="text-muted">Published articles will appear here.</p>`
	}
	left.WriteString(app.PreviewCard("home-blog", "Blog", "/blog", `<div class="home-card-content">`+reading+`</div>`))
	for _, spec := range feedServices() {
		column := &left
		if spec.Name == "markets" || spec.Name == "video" || spec.Name == "images" || spec.Name == "prayer" {
			column = &right
		}
		body := snapshot.cards[spec.Name]
		if body == "" {
			body = `<p class="text-muted">` + html.EscapeString(spec.Description) + `</p>`
		}
		column.WriteString(app.PreviewCard("home-service-"+spec.Name, spec.NavLabel(), spec.Page, `<div class="home-card-content">`+body+`</div>`))
	}
	return `<div class="page-stack"><div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div></div>`
}
