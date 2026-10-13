package agent

import (
	"fmt"
	"html"
	"mu/service/blog"
	"mu/service/events"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"mu/internal/app"
	"mu/internal/auth"
)

func setBriefPlan(owner string, enabled bool) error {
	if enabled && auth.Plan(owner) != "pro" {
		return fmt.Errorf("a daily plan is included with Pro")
	}
	return events.EditOwned(owner, func(records map[string]*events.Event) error {
		for id, old := range records {
			if old.Owner != owner || old.Kind != "brief" || BriefPeriod(old) != "morning" {
				continue
			}
			cp := *old
			cp.Plan = enabled
			records[id] = &cp
			return nil
		}
		return nil
	})
}

func Research(owner string) *events.Event {
	for _, e := range events.List(owner) {
		if e.Kind == "research" {
			return e
		}
	}
	return nil
}

func researchHTML(owner, csrf string) string {
	if auth.Plan(owner) != "pro" {
		return `<p>The day’s news and blog digests, with links to read more. Private, on a daily or weekly schedule. Available with <a href="/pricing">Pro</a>.</p>`
	}
	topic := ""
	clock, zone, frequency := "20:30", "", "daily"
	if acc, err := auth.GetAccount(owner); err == nil {
		zone = acc.Zone
	}
	e := Research(owner)
	if e != nil {
		zone, frequency = e.Zone, e.Repeat
		if e.Title == "Evening Digest" {
			topic = e.Prompt
		}
		loc, err := time.LoadLocation(zone)
		if err == nil {
			clock = e.When.In(loc).Format("15:04")
		}
	}
	options := ""
	for _, f := range []string{"daily", "weekly"} {
		selected := ""
		if f == frequency {
			selected = " selected"
		}
		options += `<option value="` + f + `"` + selected + `>` + strings.Title(f) + `</option>`
	}
	topicOptions := `<option value="">All topics</option>`
	for _, t := range blog.Topics() {
		selected := ""
		if strings.EqualFold(t, topic) {
			selected = " selected"
		}
		topicOptions += `<option value="` + html.EscapeString(t) + `"` + selected + `>` + html.EscapeString(t) + `</option>`
	}
	controls := `<button name="state" value="active">Save and enable</button>`
	if e != nil && !e.Paused {
		controls += `<button name="state" value="paused">Disable</button>`
	}
	status := "Not scheduled"
	if e != nil {
		status = strings.Title(frequency) + " at " + clock + " (" + zone + ")"
		if e.Paused {
			status = "Disabled"
		}
	}
	return `<div class="page-stack"><p>The day’s news and blog digests. Choose a topic or receive the general roundup.</p><p class="text-muted">` + status + `. No credits are used.</p><form class="form" method="POST" action="/agents?view=scheduled">` + app.CSRFField(csrf) + `<input type="hidden" name="action" value="research-schedule"><label class="field-label">Topic<select name="topic">` + topicOptions + `</select></label><label class="field-label">Frequency<select name="repeat">` + options + `</select></label><label class="field-label">Time<input type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input name="zone" data-local-timezone required value="` + html.EscapeString(zone) + `"></label><div class="form-actions">` + controls + `</div></form></div>`
}

func researchScheduleHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Forbidden(w, r, "Sign in to schedule an evening digest")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reload and try again")
		return
	}
	paused := r.FormValue("state") == "paused"
	if !paused && auth.Plan(acc.ID) != "pro" {
		app.Forbidden(w, r, "Evening Digest is included with Pro")
		return
	}
	if r.FormValue("state") != "active" && !paused {
		app.BadRequest(w, r, "Invalid schedule action")
		return
	}
	topic := strings.TrimSpace(r.FormValue("topic"))
	validTopic := topic == ""
	for _, t := range blog.Topics() {
		if strings.EqualFold(t, topic) {
			topic = t
			validTopic = true
		}
	}
	if !validTopic {
		app.BadRequest(w, r, "Choose a listed topic or all topics")
		return
	}
	repeat := r.FormValue("repeat")
	zone := r.FormValue("zone")
	clock, e2 := time.Parse("15:04", r.FormValue("clock"))
	loc, e3 := time.LoadLocation(zone)
	if (repeat != "daily" && repeat != "weekly") || e2 != nil || e3 != nil || zone == "" || zone == "Local" {
		app.BadRequest(w, r, "Choose a daily or weekly frequency, time, timezone")
		return
	}
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	err = events.EditOwned(acc.ID, func(records map[string]*events.Event) error {
		var old *events.Event
		for _, e := range records {
			if e.Owner == acc.ID && e.Kind == "research" {
				old = e
				break
			}
		}
		e := events.Event{ID: uuid.NewString(), Owner: acc.ID, Kind: "research", Created: time.Now().UTC()}
		if old != nil {
			e = *old
		}
		e.ResearchDigest, e.ResearchReport = "", ""
		e.Title, e.Prompt, e.Repeat, e.Zone, e.When, e.Paused = "Evening Digest", topic, repeat, zone, next, paused
		e.Advance = scheduledAdvance(e.Kind)
		e.Sequence++
		e.Fired = false
		e.FiredAt = time.Time{}
		records[e.ID] = &e
		return nil
	})
	if err != nil {
		app.BadRequest(w, r, "Could not save digest schedule")
		return
	}
	http.Redirect(w, r, "/agents?view=scheduled&event=research", http.StatusSeeOther)
}
