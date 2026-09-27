package agent

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"strings"
	"time"
)

func customScheduleHandler(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	if r.Method == http.MethodPost {
		if !auth.StrictCSRF(r) || !auth.CanPost(acc.ID) {
			app.Forbidden(w, r, "You cannot change schedules with this session")
			return
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		if r.FormValue("action") == "cancel-schedule" {
			var found *events.Event
			for _, e := range events.List(acc.ID) {
				if e.ID == r.FormValue("id") && e.Kind == "" && e.Prompt != "" {
					found = e
					break
				}
			}
			if found == nil {
				app.NotFound(w, r, "Schedule not found")
				return
			}
			if err := events.Cancel(acc.ID, found.ID); err != nil {
				app.BadRequest(w, r, err.Error())
				return
			}
			http.Redirect(w, r, "/agents?view=scheduled", http.StatusSeeOther)
			return
		}
		title, prompt := strings.TrimSpace(r.FormValue("title")), strings.TrimSpace(r.FormValue("prompt"))
		zone := r.FormValue("zone")
		loc, err := time.LoadLocation(zone)
		if err != nil || zone == "" || zone == "Local" {
			app.BadRequest(w, r, "Choose a valid timezone")
			return
		}
		when, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("when"), loc)
		repeat := r.FormValue("repeat")
		if err != nil || when.Format("2006-01-02T15:04") != r.FormValue("when") || !when.After(time.Now()) || title == "" || len(title) > 1000 || prompt == "" || len(prompt) > 16000 || (repeat != "" && repeat != "daily" && repeat != "weekly" && repeat != "monthly") {
			app.BadRequest(w, r, "Enter a title, instructions and a future date and time")
			return
		}
		if _, err := events.CreateStanding(acc.ID, title, when, "", 0, repeat, prompt, zone); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		http.Redirect(w, r, "/agents?view=scheduled", http.StatusSeeOther)
		return
	}
	token := auth.CSRFToken(r)
	body := `<div class="page-stack">` + app.PageControls(agentsDescription, scheduledTabs(true), "") + `<section class="section-card section-stack"><h2>New schedule</h2><p>Tell Micro what to do and when. The result arrives in your inbox. Assistant replies and paid tools use your credits.</p><form method="POST" action="/agents?view=scheduled" class="form">` + app.CSRFField(token) + `<input type="hidden" name="action" value="create-schedule">` +
		app.Field{Name: "title", Label: "Title", Required: true, Max: 1000}.HTML() +
		app.Field{Name: "prompt", Label: "What should Micro do?", Placeholder: "Check the latest developments on a topic and send me a summary", Required: true, Rows: 3, Max: 16000}.HTML() +
		app.Field{Name: "when", Label: "Date and time", Type: "datetime-local", Required: true}.HTML() +
		`<label class="field-label">Timezone<input name="zone" data-local-timezone required value="` + html.EscapeString(acc.Zone) + `" placeholder="Europe/London"></label>` +
		app.Field{Name: "repeat", Label: "Repeat", Options: []app.Option{{Value: "", Label: "Once"}, {Value: "daily", Label: "Daily"}, {Value: "weekly", Label: "Weekly"}, {Value: "monthly", Label: "Monthly"}}}.HTML() +
		`<div class="form-actions"><button type="submit">Create schedule</button><a href="/agents?view=scheduled">Cancel</a></div></form></section></div>`
	app.Respond(w, r, app.Response{Title: "Agents", HTML: body})
}

func customSchedulesHTML(owner, token string) string {
	var b strings.Builder
	for _, e := range events.List(owner) {
		if e.Kind != "" || e.Prompt == "" || e.Fired {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(`<section class="section-stack"><h2>Your schedules</h2><div class="collection-list">`)
		}
		state := "Scheduled"
		if e.Paused {
			state = "Paused"
		}
		fmt.Fprintf(&b, `<article class="record-card section-stack"><h3>%s</h3><p class="text-muted">%s · %s</p><p>%s</p><form method="POST" action="/agents?view=scheduled" class="form-actions">%s<input type="hidden" name="action" value="cancel-schedule"><input type="hidden" name="id" value="%s"><button type="submit">Cancel schedule</button></form></article>`, html.EscapeString(e.Title), state, html.EscapeString(events.Describe(e)), html.EscapeString(e.Prompt), app.CSRFField(token), html.EscapeString(e.ID))
	}
	if b.Len() > 0 {
		b.WriteString(`</div></section>`)
	}
	return b.String()
}
