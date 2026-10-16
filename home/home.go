// Package home owns the authenticated personal overview and saved collections.
package home

import (
	"fmt"
	"html"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"

	"mu/account"
	"mu/agent"
	"mu/agent/brief"
	"mu/agent/work"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
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
		if quota.Charging() && !acc.Admin && !acc.Agent && quota.Available(acc.ID) < quota.OperationCost(quota.OpAgentRun) {
			body += `<section class="record-card"><h2>Activate your assistant</h2><p>Choose a monthly plan or add prepaid credits for personal chat and tools.</p><div class="form-actions"><a class="btn" href="/pricing">Choose a plan</a><a href="/account/topup">Add credit</a></div></section>`
		} else {
			body += `<div>` + agent.Prompt(acc.ID) + `</div>`
		}
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
	left.WriteString(conversationsCard(acc.ID))
	if preview := inbox.Preview(acc.ID); preview != "" {
		left.WriteString(app.PreviewCard("home-inbox", "Inbox", "/inbox", preview))
	}
	right.WriteString(events.Preview(acc.ID, events.CachedOverview(acc.ID)))
	right.WriteString(work.ScheduledCard(acc.ID))
	columns := `<div class="dashboard-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div>`
	if left.Len() == 0 {
		columns = `<div class="page-col">` + right.String() + `</div>`
	}
	return `<div class="page-stack">` + columns + `</div>`
}

func feedHTML(snapshot overviewSnapshot, owner string) string {
	var left, right strings.Builder
	left.WriteString(shortBrief())
	reading := blog.LatestPreview(owner)
	if reading == "" {
		reading = `<p class="text-muted">Published articles will appear here.</p>`
	}
	left.WriteString(app.PreviewCard("home-blog", "Blog", "/blog", `<div class="home-card-content">`+reading+`</div>`))
	for _, spec := range feedServices() {
		column := &left
		if spec.Name == "video" || spec.Name == "images" || spec.Name == "prayer" || spec.Name == "markets" {
			column = &right
		}
		body := snapshot.cards[spec.Name]
		if body == "" {
			body = `<p class="text-muted">` + html.EscapeString(spec.Description) + `</p>`
		}
		column.WriteString(app.PreviewCard("home-service-"+spec.Name, spec.NavLabel(), spec.Page, `<div class="home-card-content">`+body+`</div>`))
	}
	return `<div class="page-stack"><div class="dashboard-columns feed-columns"><div class="page-stack">` + left.String() + `</div><div class="page-stack">` + right.String() + `</div></div></div>`
}

func conversationsCard(owner string) string {
	var b strings.Builder
	count := 0
	for _, conversation := range thread.List(owner, 0) {
		if conversation.Client != thread.WebClient {
			continue
		}
		title := conversation.Subject
		if title == "" {
			title = "Conversation with Micro"
		}
		target := agent.Path(owner, conversation.Agent) + "?session=" + url.QueryEscape(conversation.ID)
		b.WriteString(`<a class="collection-item" href="` + html.EscapeString(target) + `"><span>` + html.EscapeString(title) + `</span><time class="text-muted" datetime="` + conversation.Updated.Format("2006-01-02T15:04:05Z07:00") + `">` + conversation.Updated.Format("2 Jan · 15:04") + `</time></a>`)
		count++
		if count == 5 {
			break
		}
	}
	body := `<p class="text-muted">Your recent conversations with Micro will appear here.</p><a href="/agent/micro">Start a conversation</a>`
	if count > 0 {
		body = `<div class="collection-list">` + b.String() + `</div>`
	}
	return app.PreviewCard("home-conversations", "Conversations", "/agent/micro", body)
}
