package work

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/events"
	"mu/service/tasks"
	"net/http"
	"net/url"
	"time"
)

type scheduleGroup struct {
	ID, Title, Status, Label string
	Created, Updated         time.Time
	Latest                   *tasks.Task
	Schedule                 *events.Event
}

func scheduledGroups(owner string) []scheduleGroup {
	groups := map[string]*scheduleGroup{}
	for _, e := range events.List(owner) {
		if e.Advance.Recipient != "agent" || e.Advance.Minutes == 0 {
			continue
		}
		label, status := "Scheduled", "todo"
		if e.Paused {
			label, status = "Paused", "canceled"
		}
		groups[e.ID] = &scheduleGroup{ID: e.ID, Title: e.Title, Status: status, Label: label, Created: e.Created, Updated: e.Created, Schedule: e}
	}
	for _, t := range tasks.LatestOccurrences(owner) {
		id := t.Occurrence.Schedule
		g := groups[id]
		if g == nil {
			g = &scheduleGroup{ID: id, Title: t.Title, Created: t.Created}
			groups[id] = g
		}
		g.Latest, g.Updated = t, t.Updated
		g.Status, g.Label = occurrenceStatus(t, g.Schedule)
	}
	out := []scheduleGroup{}
	for _, g := range groups {
		out = append(out, *g)
	}
	return out
}

func occurrenceStatus(t *tasks.Task, e *events.Event) (string, string) {
	o := t.Occurrence
	if o == nil {
		return t.Status, state(t.Status)
	}
	if o.State != "done" && o.State != "delivery_failed" && (e == nil || e.Paused || fmt.Sprint(e.Sequence) != o.Revision) {
		return "canceled", "Canceled"
	}
	switch o.State {
	case "started":
		if _, ok := preparingOccurrences.Load(o.Key); !ok {
			return "blocked", "Interrupted"
		}
		return "doing", "Preparing"
	case "ready":
		if o.Failure != "" {
			return "failed", "Preparation failed"
		}
		return "doing", "Ready for delivery"
	case "delivering":
		return "blocked", "Delivery unconfirmed"
	case "delivery_failed":
		return "failed", "Delivery failed"
	case "canceled":
		return "canceled", "Canceled"
	case "done":
		if o.Failure != "" {
			return "failed", "Preparation failed · notice sent"
		}
		return "done", "Delivered"
	}
	return t.Status, state(t.Status)
}

func scheduleHistory(w http.ResponseWriter, r *http.Request, acc *auth.Account, id string) {
	var found *scheduleGroup
	for _, g := range scheduledGroups(acc.ID) {
		if g.ID == id {
			cp := g
			found = &cp
			break
		}
	}
	if found == nil {
		app.NotFound(w, r, "Scheduled work not found")
		return
	}
	runs := tasks.Occurrences(acc.ID, id)
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"schedule": found.Schedule, "runs": runs})
		return
	}
	zone := time.UTC
	if loc, err := time.LoadLocation(acc.Zone); err == nil {
		zone = loc
	}
	body := `<div class="page-stack"><p><a href="/work">Back to work</a></p><section class="section-card section-stack"><h2>` + html.EscapeString(found.Title) + `</h2><p>Scheduled work · one entry, with a separate run for each occurrence.</p>`
	if e := found.Schedule; e != nil {
		if e.Zone != "" {
			if loc, err := time.LoadLocation(e.Zone); err == nil {
				zone = loc
			}
		}
		if !e.Paused && !e.Fired {
			body += `<p>Next delivery: ` + html.EscapeString(e.When.In(zone).Format("Mon 2 Jan 2006, 15:04 MST")) + `</p>`
		}
		body += `<p><a href="/agents?view=scheduled">Manage schedule</a></p>`
	}
	body += `</section><section class="section-stack"><h2>Recent runs</h2>`
	pager := app.Paginate(r, len(runs), 25)
	for _, t := range runs[pager.From:pager.To] {
		_, label := occurrenceStatus(t, found.Schedule)
		body += `<article class="record-card section-stack"><h3><a href="/work?id=` + url.QueryEscape(t.ID) + `">` + html.EscapeString(t.Due.In(zone).Format("Mon 2 Jan 2006, 15:04 MST")) + `</a></h3><div class="metadata-row"><span>` + html.EscapeString(label) + `</span></div></article>`
	}
	if len(runs) == 0 {
		body += `<p>No runs yet.</p>`
	}
	body += pager.Nav("/work?schedule="+url.QueryEscape(id)) + `</section></div>`
	app.Respond(w, r, app.Response{Title: "Scheduled work", HTML: body})
}

func occurrenceDetail(t *tasks.Task) string {
	schedule := scheduleFor(t.Owner, t.Occurrence.Schedule)
	_, label := occurrenceStatus(t, schedule)
	loc := time.UTC
	if acc, err := auth.GetAccount(t.Owner); err == nil {
		if l, e := time.LoadLocation(acc.Zone); e == nil {
			loc = l
		}
	}
	if schedule != nil {
		if l, e := time.LoadLocation(schedule.Zone); e == nil {
			loc = l
		}
	}
	body := `<div class="page-stack"><p><a href="/work?schedule=` + url.QueryEscape(t.Occurrence.Schedule) + `">Back to run history</a></p><section class="section-card section-stack"><h2>` + html.EscapeString(t.Title) + `</h2><p>` + html.EscapeString(label) + `</p><p>Delivery time: ` + html.EscapeString(t.Due.In(loc).Format(time.RFC1123)) + `</p>`
	if th := thread.ByRef(t.Owner, t.Occurrence.MessageID); t.Occurrence.MessageID != "" && th != nil {
		body += `<p><a href="/inbox?id=` + url.QueryEscape(th.ID) + `">Open in Inbox</a></p>`
	}
	body += `<div class="metadata-row"><span>Execution: ` + html.EscapeString(t.Occurrence.Execution) + `</span><span>Delivery: ` + html.EscapeString(t.Occurrence.Delivery) + `</span></div>`
	if t.Occurrence.Failure != "" {
		body += `<p>` + html.EscapeString(t.Occurrence.Failure) + `</p>`
	}
	body += `</section>`
	if t.Result != "" {
		body += `<section class="section-card section-stack"><h2>Result</h2>` + app.RenderString(t.Result) + `</section>`
	}
	return body + `</div>`
}

func scheduleFor(owner, id string) *events.Event {
	for _, e := range events.List(owner) {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// ScheduledCounts supplies Home with one count per schedule, never per past run.
func ScheduledCounts(owner string) (active, attention int) {
	for _, g := range scheduledGroups(owner) {
		switch g.Status {
		case "doing":
			active++
		case "blocked", "failed":
			attention++
		}
	}
	return
}
