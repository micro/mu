// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"fmt"
	"html"
	"net/http"
	"sort"
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
		content = feedHTML(snapshot, acc.ID)
		source = "/home?view=feed"
	}
	// Refresh only the selected view; never replace the prompt or a draft.
	if r.URL.Query().Get("view") == "overview" || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"html": content, "pending": pending, "weather": weatherLine(acc.ID)})
		return
	}
	now := account.LocalNow(acc.ID)
	header := `<div data-home-overview class="home-date"><time datetime="` + now.Format("2006-01-02") + `">` + now.Format("Monday, 2 January") + `</time><span id="home-weather">` + weatherLine(acc.ID) + `</span></div>`
	body := `<div class="section-body"><div data-home-overview>` + tabs(view) + `</div>`
	if view == "overview" {
		body += `<div>` + agent.Prompt(acc.ID) + `</div>`
	} else {
		body += `<p>News, reading and live updates from your services.</p>`
	}
	body += `</div><div data-home-overview id="home-overview-content" data-source="` + html.EscapeString(source) + `" data-pending="` + fmt.Sprint(pending) + `">` + content + `</div>`
	app.Respond(w, r, app.Response{Title: "Home", HTML: header + `<div class="page-stack">` + body + `</div>`})
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
	return app.PreviewCard("home-brief", "Brief", "/brief?id="+entry.ID(), `<p class="home-summary">`+html.EscapeString(line)+`</p>`)
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
	services := service.Pinned(acc.PinnedServices())
	if len(services) == 0 {
		services = service.Pinned([]string{"news", "markets", "video", "blog"})
	}
	sort.SliceStable(services, func(i, j int) bool {
		return strings.ToLower(services[i].NavLabel()) < strings.ToLower(services[j].NavLabel())
	})
	for _, spec := range services {
		pins.WriteString(`<a href="` + html.EscapeString(spec.Page) + `"><span class="service-shortcut-icon"><img src="/` + html.EscapeString(spec.NavIcon()) + `?` + app.Version + `" width="28" height="28" alt=""></span><span>` + html.EscapeString(spec.NavLabel()) + `</span></a>`)
	}
	pinned := ""
	if pins.Len() > 0 {
		pinned = `<section class="shortcut-section" aria-labelledby="pinned-services-title"><h2 id="pinned-services-title">Pinned</h2><nav class="service-shortcuts" aria-labelledby="pinned-services-title">` + pins.String() + `</nav></section>`
	}
	columns := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		columns = `<div class="page-col">` + right.String() + `</div>`
	}
	return `<div class="page-stack">` + pinned + columns + `</div>`
}

func feedHTML(snapshot overviewSnapshot, owner string) string {
	var left, right strings.Builder
	reading := blog.PrivatePreview(owner) + blog.Preview()
	if reading == "" {
		reading = `<p class="text-muted">Published articles will appear here.</p>`
	}
	left.WriteString(app.PreviewCard("home-blog", "Blog", "/blog", `<div class="home-card-content">`+reading+`</div>`))
	for _, spec := range feedServices() {
		column := &left
		if spec.Name == "video" || spec.Name == "images" || spec.Name == "prayer" {
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
