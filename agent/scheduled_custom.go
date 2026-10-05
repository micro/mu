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
	"time"
)

func customScheduleHandler(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var existing *events.Event
	id := r.URL.Query().Get("event")
	if r.Method == http.MethodPost && r.FormValue("action") == "update-schedule" {
		id = r.FormValue("id")
	}
	if id != "" {
		for _, e := range events.List(acc.ID) {
			if e.ID == id && e.Kind == "" && e.Prompt != "" {
				existing = e
				break
			}
		}
		if existing == nil {
			app.NotFound(w, r, "Scheduled event not found")
			return
		}
	}

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
		advance := events.Advance{}
		if r.FormValue("prepare") == "yes" {
			advance = events.Advance{Minutes: 10, Recipient: "agent"}
		}
		if existing != nil {
			err = events.EditOwned(acc.ID, func(records map[string]*events.Event) error {
				e := records[existing.ID]
				if e == nil || e.Kind != "" || e.Prompt == "" {
					return fmt.Errorf("scheduled event not found")
				}
				e.Title, e.Prompt, e.When, e.Zone, e.Repeat, e.Advance = title, prompt, when, zone, repeat, advance
				e.Fired, e.FiredAt = false, time.Time{}
				e.Sequence++
				return nil
			})
		} else {
			_, err = events.CreateScheduled(acc.ID, title, when, "", 0, repeat, prompt, advance, zone)
		}
		if err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		destination := "/agents?view=scheduled"
		if existing != nil {
			destination += "&event=" + url.QueryEscape(existing.ID)
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	token := auth.CSRFToken(r)
	title, prompt, when, zone, repeat, checked := "", "", "", acc.Zone, "", ""
	heading, action, button := "New event", "create-schedule", "Create event"
	hidden, cancel := "", ""
	if existing != nil {
		title, prompt, zone, repeat = existing.Title, existing.Prompt, existing.Zone, existing.Repeat
		loc, err := time.LoadLocation(zone)
		if err != nil {
			loc = time.UTC
		}
		when = existing.When.In(loc).Format("2006-01-02T15:04")
		if existing.Advance.Recipient == "agent" && existing.Advance.Minutes > 0 {
			checked = " checked"
		}
		heading, action, button = "Scheduled event", "update-schedule", "Save"
		hidden = `<input type="hidden" name="id" value="` + html.EscapeString(existing.ID) + `">`
		cancel = `<form method="POST" action="/agents?view=scheduled" class="form-actions">` + app.CSRFField(token) + hidden + `<input type="hidden" name="action" value="cancel-schedule"><button>Cancel event</button></form>`
	}
	body := `<div class="page-stack">` + app.PageControls(agentsDescription, scheduledTabs(true), "") + `<section class="section-card section-stack"><h2>` + heading + `</h2><p>Tell Micro what to do and when. The result arrives in your inbox. Assistant replies and paid tools use your credits.</p><form method="POST" action="/agents?view=scheduled" class="form">` + app.CSRFField(token) + hidden + `<input type="hidden" name="action" value="` + action + `">` +
		app.Field{Name: "title", Label: "Title", Value: title, Required: true, Max: 1000}.HTML() +
		app.Field{Name: "prompt", Label: "What should Micro do?", Value: prompt, Required: true, Rows: 3, Max: 16000}.HTML() +
		`<label><input type="checkbox" name="prepare" value="yes"` + checked + `> Prepare a report ahead of delivery</label><p class="text-muted">For reading and summaries: Micro starts 10 minutes early using read-only tools, then delivers at the selected time. Leave unchecked for tasks that send messages or change records.</p>` +
		app.Field{Name: "when", Label: "Date and time", Type: "datetime-local", Value: when, Required: true}.HTML() +
		`<label class="field-label">Timezone<input name="zone" data-local-timezone required value="` + html.EscapeString(zone) + `" placeholder="Europe/London"></label>` +
		app.Field{Name: "repeat", Label: "Repeat", Value: repeat, Options: []app.Option{{Value: "", Label: "Once"}, {Value: "daily", Label: "Daily"}, {Value: "weekly", Label: "Weekly"}, {Value: "monthly", Label: "Monthly"}}}.HTML() +
		`<div class="form-actions"><button type="submit">` + button + `</button><a href="/agents?view=scheduled">Back</a></div></form>` + cancel + `</section></div>`
	app.Respond(w, r, app.Response{Title: "Agents", HTML: body})
}

func customSchedulesHTML(owner, token string) string {
	var b strings.Builder
	for _, e := range events.List(owner) {
		if e.Kind != "" || e.Prompt == "" || e.Fired {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(`<section class="section-stack"><h2>Your scheduled events</h2><div class="collection-list">`)
		}
		b.WriteString(scheduleCard(e.ID, e.Title, e.Prompt, e))
	}
	if b.Len() > 0 {
		b.WriteString(`</div></section>`)
	}
	return b.String()
}
