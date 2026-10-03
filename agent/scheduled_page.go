package agent

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

const agentsDescription = "Talk to an agent, create your own, or choose scheduled updates from Micro."

func scheduledTabs(scheduled bool) string {
	active := "agents"
	if scheduled {
		active = "scheduled"
	}
	return app.ViewNavigation("Agent views", active, []app.ViewLink{
		{Key: "agents", Label: "Agents", URL: "/agents"},
		{Key: "scheduled", Label: "Scheduled", URL: "/agents?view=scheduled"},
	}, false)
}

func scheduledHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if (r.Method == http.MethodGet && r.URL.Query().Get("new") == "1") || (r.Method == http.MethodPost && (r.FormValue("action") == "create-schedule" || r.FormValue("action") == "cancel-schedule")) {
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
	token := auth.CSRFToken(r)
	body := `<div class="page-stack">` + app.PageControls(agentsDescription, scheduledTabs(true), `<div class="form-actions"><a class="btn" href="/agents?view=scheduled&amp;new=1">New task</a></div>`) + `<div class="card-grid">` + briefPeriodHTML(acc.ID, token, "morning") + checkinHTML(acc.ID, token) + momentHTML(acc.ID, token) + `<section id="research" class="section-card section-stack"><h2>Evening Reading</h2>` + researchHTML(acc.ID, token) + `</section></div>` + customSchedulesHTML(acc.ID, token) + `</div>`
	app.Respond(w, r, app.Response{Title: "Agents", HTML: body})
}
