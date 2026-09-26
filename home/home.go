// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/account"
	"mu/agent"
	"mu/agent/brief"
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
		resume = `<p class="home-resume" data-home-overview><a href="` + html.EscapeString(agent.Path(acc.ID, th.Agent)+"?session="+url.QueryEscape(th.ID)) + `">Continue: ` + html.EscapeString(th.Subject) + `</a></p>`
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

// Personal facts and the cached world summary. Rendering never calls a model.
func shortBrief(owner string) string {
	var parts []string
	if n, newest := inbox.Waiting(owner); n > 0 {
		noun := "conversations"
		if n == 1 {
			noun = "conversation"
		}
		text := fmt.Sprintf(`<a href="/inbox?filter=unread">%d unread %s</a>`, n, noun)
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
	if line := nextEvent(owner); line != "" {
		parts = append(parts, line)
	}
	if line := brief.Line(); line != "" {
		parts = append(parts, html.EscapeString(line))
	}
	if len(parts) == 0 {
		return ""
	}
	return `<section class="section-card" aria-labelledby="home-brief-title"><div class="section-card-head"><h2 id="home-brief-title">Brief</h2></div><p class="home-summary">` + strings.Join(parts, " ") + `</p></section>`
}

// Read only the local calendar and its already cached external preview.
func nextEvent(owner string) string {
	now := account.LocalNow(owner)
	var when time.Time
	title := ""
	consider := func(name string, at time.Time) {
		at = at.In(now.Location())
		if at.After(now) && at.Format("2006-01-02") == now.Format("2006-01-02") && (when.IsZero() || at.Before(when)) {
			title, when = name, at
		}
	}
	for _, e := range events.Upcoming(owner) {
		if e.Kind != "brief" && e.Prompt == "" {
			consider(e.Title, e.When)
		}
	}
	for _, e := range events.CachedOverview(owner) {
		if !e.AllDay {
			consider(e.Title, e.Start)
		}
	}
	if title == "" {
		return ""
	}
	return `<a href="/events">` + html.EscapeString(title) + `</a> at ` + when.Format("15:04") + `.`
}

func overviewHTML(r *http.Request, acc *auth.Account, snapshot overviewSnapshot) string {
	var left, right strings.Builder
	if preview := inbox.Preview(acc.ID); preview != "" {
		left.WriteString(app.PreviewCard("home-inbox", "Inbox", "/inbox", preview))
	}
	right.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	right.WriteString(`<section class="record-card"><div class="section-card-head"><h2>Saved items</h2></div><nav class="form-actions" aria-label="Saved items"><a href="/docs">Docs</a><a href="/files">Files</a><a href="/notes">Notes</a><a href="/bookmarks">Bookmarks</a><a href="/home/apps">My apps</a></nav></section>`)
	if len(snapshot.apps) > 0 {
		right.WriteString(`<section class="record-card"><div class="section-card-head"><h2>My apps</h2><a href="/home/apps">View all</a></div><div class="collection-list">`)
		for i, a := range snapshot.apps {
			if i == 3 {
				break
			}
			right.WriteString(`<a class="collection-item" href="/apps/` + url.PathEscape(a.Slug) + `">` + html.EscapeString(a.Name) + `</a>`)
		}
		right.WriteString(`</div></section>`)
	}
	for i, spec := range service.Pinned(acc.PinnedServices()) {
		column := &left
		if i%2 != 0 {
			column = &right
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
	return `<div class="page-col">` + shortBrief(acc.ID) + `<nav class="form-actions" aria-label="Home services"><a href="/services">Pin services to Home</a></nav>` + columns + `</div>`
}
