package events

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves the /events page: schedule a reminder, see what's upcoming,
// and cancel. GET with an Accept: application/json header returns the caller's
// upcoming events as JSON.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.FormValue("action") == "brief-schedule" {
		briefScheduleHandler(w, r)
		return
	}
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Redirect(w, r, "/login?next=/events", http.StatusSeeOther)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "cancel":
			Cancel(owner, r.FormValue("id"))
		case "create":
			when, err := parseWhen(r.FormValue("when"))
			if err == nil {
				Create(owner, r.FormValue("title"), when, r.FormValue("note"))
			}
		}
		http.Redirect(w, r, "/events", http.StatusSeeOther)
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		app.RespondJSON(w, Upcoming(owner))
		return
	}

	if id := r.URL.Query().Get("id"); id != "" {
		detailHandler(w, r, owner, id)
		return
	}
	csrf := auth.CSRFToken(r)
	var b strings.Builder

	if r.URL.Query().Get("new") == "1" {
		app.Respond(w, r, app.Response{Title: "New event", HTML: eventForm(csrf)})
		return
	}
	b.WriteString(`<div class="page-col page-stack"><div class="page-action"><a class="btn" href="/events?new=1">New</a></div>`)
	b.WriteString(briefScheduleHTML(owner, csrf))

	up := Upcoming(owner)

	if len(up) == 0 {
		b.WriteString(`<p class="text-muted text-base">Nothing scheduled. Choose New to add an event, or ask the agent: <em>"remind me to call the dentist tomorrow at 3pm"</em>.</p>`)
	} else {
		b.WriteString(`<h3 >Upcoming</h3>`)
		b.WriteString(`<div class="compact-list">`)
		for _, e := range up {
			if e.Kind != "brief" {
				b.WriteString(eventRow(e, csrf))
			}
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Events", Description: "Your scheduled reminders and events", HTML: b.String()})
}

func eventRow(e *Event, csrf string) string {
	note := ""
	if e.Note != "" {
		note = `<div class="text-xs text-muted">` + html.EscapeString(e.Note) + `</div>`
	}
	return fmt.Sprintf(`<div class="list-row compact-row">
<div class="grow">
  <a class="text-base no-underline" href="%s">%s</a>
  <div class="metadata-row">%s</div>
  %s
</div>
<form method="POST" action="/events" class="form-action m-0">
  <input type="hidden" name="_csrf" value="%s">
  <input type="hidden" name="action" value="cancel">
  <input type="hidden" name="id" value="%s">
  <button type="submit" title="Cancel" class="btn-quiet">&times;</button>
</form>
</div>`,
		html.EscapeString(eventURL(e.ID)),
		html.EscapeString(e.Title),
		e.When.Local().Format("Mon 2 Jan, 15:04"),
		note,
		html.EscapeString(csrf),
		html.EscapeString(e.ID),
	)
}
