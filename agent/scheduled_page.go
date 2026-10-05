package agent

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"net/url"
	"strings"
)

const agentsDescription = "Talk to an agent, create your own, or choose scheduled updates from Micro."

func scheduledTabs(scheduled bool) string {
	active := "agents"
	if scheduled {
		active = "scheduled"
	}
	return app.AgentViews(active)
}

func scheduledHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if (r.Method == http.MethodGet && r.URL.Query().Get("new") == "1") || (r.Method == http.MethodPost && (r.FormValue("action") == "create-schedule" || r.FormValue("action") == "cancel-schedule" || r.FormValue("action") == "update-schedule")) {
		customScheduleHandler(w, r, acc)
		return
	}
	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "brief-schedule":
			briefScheduleHandler(w, r)
		case "checkin-schedule":
			checkinScheduleHandler(w, r)
		case "moment-schedule":
			invitationScheduleHandler(w, r, "moment")
		case "research-schedule":
			researchScheduleHandler(w, r)
		default:
			app.BadRequest(w, r, "Unknown scheduled action")
		}
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"brief": Brief(acc.ID), "checkin": Checkin(acc.ID), "moment": Moment(acc.ID), "research": Research(acc.ID)})
		return
	}
	auth.SetCSRFCookie(w, r)
	token := auth.CSRFToken(r)
	if id := r.URL.Query().Get("event"); id != "" {
		var content string
		switch id {
		case "brief":
			content = briefPeriodHTML(acc.ID, token, "morning")
		case "checkin":
			content = checkinHTML(acc.ID, token)
		case "moment":
			content = momentHTML(acc.ID, token)
		case "research":
			content = `<section class="section-card section-stack"><h2>Evening Reading</h2>` + researchHTML(acc.ID, token) + `</section>`
		default:
			customScheduleHandler(w, r, acc)
			return
		}
		body := `<div class="page-stack">` + app.PageControls(agentsDescription, scheduledTabs(true), "") + content + `</div>`
		app.Respond(w, r, app.Response{Title: "Scheduled event", HTML: body})
		return
	}
	body := `<div class="page-stack">` + app.PageControls(agentsDescription, scheduledTabs(true), `<div class="form-actions"><a class="btn" href="/agents?view=scheduled&amp;new=1">New event</a></div>`) + `<div class="card-grid">`
	for _, item := range []struct {
		id, title, description string
		event                  *events.Event
	}{
		{"brief", "Morning Brief", "Your day, useful headlines and a reminder.", Brief(acc.ID)},
		{"checkin", "Daily Checkin", "How's it going?", Checkin(acc.ID)},
		{"moment", "Take a moment", "A gentle invitation to pause, stretch or step outside.", Moment(acc.ID)},
		{"research", "Evening Reading", "A private reading on your chosen topic.", Research(acc.ID)},
	} {
		body += scheduleCard(item.id, item.title, item.description, item.event)
	}
	body += `</div>` + customSchedulesHTML(acc.ID, token) + `</div>`
	app.Respond(w, r, app.Response{Title: "Agents", HTML: body})
}

func scheduleCard(id, title, description string, e *events.Event) string {
	status := "Not scheduled"
	if e != nil {
		status = e.LocalTime().Format("Mon 2 Jan, 15:04 MST")
		if e.Repeat != "" {
			status = strings.Title(e.Repeat) + " at " + e.LocalTime().Format("15:04")
		}
		if e.Paused {
			status = "Disabled"
		}
	}
	if id == "research" && e != nil && !e.Paused {
		status += fmt.Sprintf(" · Up to %d credits", e.MaxCredits)
	}
	anchor := id
	if id == "brief" {
		anchor = "morning-brief"
	}
	return `<a id="` + html.EscapeString(anchor) + `" class="record-card section-stack" href="/agents?view=scheduled&amp;event=` + url.QueryEscape(id) + `"><h2>` + html.EscapeString(title) + `</h2><p>` + html.EscapeString(description) + `</p><p class="text-muted">` + html.EscapeString(status) + `</p></a>`
}
