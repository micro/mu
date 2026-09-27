package agent

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

func scheduledTabs(scheduled bool) string {
	return `<nav class="page-menu" aria-label="Agents">` + app.PillLink("Agents", "/agents", !scheduled) + app.PillLink("Scheduled", "/agents?view=scheduled", scheduled) + `</nav>`
}

func scheduledHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "brief-schedule":
			briefScheduleHandler(w, r)
		case "checkin-schedule":
			checkinScheduleHandler(w, r)
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
		app.RespondJSON(w, map[string]any{"brief": Brief(acc.ID), "checkin": Checkin(acc.ID), "research": Research(acc.ID)})
		return
	}
	token := auth.CSRFToken(r)
	body := `<div class="page-stack"><p class="text-muted">Choose the updates Micro prepares for you and when they arrive.</p>` + scheduledTabs(true) + briefPeriodHTML(acc.ID, token, "morning") + checkinHTML(acc.ID, token) + `<section id="research" class="section-card section-stack"><h2>Evening Research</h2>` + researchHTML(acc.ID, token) + `</section></div>`
	app.Respond(w, r, app.Response{Title: "Agents", HTML: body})
}
