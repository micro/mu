// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/account"
	"mu/agent"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/events"
	"mu/service/tasks"
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
	snapshot, pending := overview(acc)
	content := overviewHTML(r, acc, snapshot)
	// The first visit can fill its cards after paint without replacing the prompt
	// or draft. Subsequent visits render the cached overview immediately.
	if r.URL.Query().Get("view") == "overview" {
		app.RespondJSON(w, map[string]any{"html": content, "pending": pending, "weather": weatherLine(acc.ID)})
		return
	}
	resume := ""
	if th := agent.RecentConversation(acc.ID, ""); th != nil {
		resume = `<p class="home-resume"><a href="` + html.EscapeString(agent.Path(acc.ID, th.Agent)+"?session="+url.QueryEscape(th.ID)) + `">Continue: ` + html.EscapeString(th.Subject) + `</a></p>`
	}
	body := `<div data-home-overview>` + `<div class="home-date"><time datetime="` + account.LocalNow(acc.ID).Format("2006-01-02") + `">` + account.LocalNow(acc.ID).Format("Monday, 2 January") + `</time>` + `<span id="home-weather">` + weatherLine(acc.ID) + `</span></div>` + `</div>` + agent.Prompt(acc.ID) + resume + `<div data-home-overview id="home-overview-content" data-pending="` + fmt.Sprint(pending) + `">` + content + `</div>`
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

// Local facts, not another scheduled brief or a page-load model call.
func shortBrief(owner string) string {
	var parts []string
	if n, newest := inbox.Waiting(owner); n > 0 {
		noun := "conversations"
		if n == 1 {
			noun = "conversation"
		}
		text := fmt.Sprintf(`<a href="/inbox">%d %s</a> waiting`, n, noun)
		if newest != "" && !strings.EqualFold(newest, "You") {
			text += ", the newest from " + html.EscapeString(newest)
		}
		parts = append(parts, text+".")
	}
	if doing := tasks.List(owner, tasks.StatusDoing); len(doing) > 0 {
		noun := "tasks"
		if len(doing) == 1 {
			noun = "task"
		}
		parts = append(parts, fmt.Sprintf(`<a href="/work">%d %s</a> in progress.`, len(doing), noun))
	}
	if len(parts) == 0 {
		return ""
	}
	return `<h2>Brief</h2><p class="home-summary">` + strings.Join(parts, " ") + `</p>`
}

func overviewHTML(r *http.Request, acc *auth.Account, snapshot overviewSnapshot) string {
	var b strings.Builder
	b.WriteString(shortBrief(acc.ID))
	b.WriteString(`<div class="home-grid"><div>`)
	b.WriteString(`<section class="record-card"><div class="home-card-heading"><h2>My apps</h2><a href="/home/apps">View all</a></div><div class="collection-list">`)
	for i, a := range snapshot.apps {
		if i == 3 {
			break
		}
		b.WriteString(`<a class="collection-item" href="/apps/` + url.PathEscape(a.Slug) + `">` + html.EscapeString(a.Name) + `</a>`)
	}
	if len(snapshot.apps) == 0 {
		b.WriteString(`<p class="text-muted">Open your apps or ask Micro to build one.</p>`)
	}
	b.WriteString(`</div></section>`)

	b.WriteString(`<section class="record-card"><div class="home-card-heading"><h2>Your things</h2></div><div class="form-actions"><a href="/docs">Docs</a><a href="/files">Files</a><a href="/notes">Notes</a><a href="/bookmarks">Bookmarks</a></div></section>`)
	b.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	if preview := inbox.Preview(acc.ID); preview != "" {
		b.WriteString(app.PreviewCard("home-inbox", "Inbox", "/inbox", preview))
	}
	b.WriteString(`</div><div><div class="home-card-heading"><h2>Services</h2><a href="/services">Choose services</a></div>`)
	for _, spec := range service.Pinned(acc.PinnedServices()) {
		b.WriteString(`<section class="record-card"><div class="home-card-heading"><h2><a href="` + html.EscapeString(spec.Page) + `">` + html.EscapeString(spec.NavLabel()) + `</a></h2></div>`)
		if card := snapshot.cards[spec.Name]; card != "" {
			b.WriteString(`<div class="home-card-content">` + card + `</div>`)
		} else {
			b.WriteString(`<p class="text-muted">` + html.EscapeString(spec.Description) + `</p>`)
		}
		b.WriteString(`</section>`)
	}
	if len(acc.PinnedServices()) == 0 {
		b.WriteString(`<p class="text-muted">Pin services to see them here.</p>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}
