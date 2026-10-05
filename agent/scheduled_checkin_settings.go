package agent

import (
	"fmt"
	"html"
	"mu/service/events"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"mu/internal/app"
	"mu/internal/auth"
)

// Checkin returns the owner's optional check-in, independently of their brief.
func Checkin(owner string) *events.Event {
	return invitation(owner, "checkin")
}

func Moment(owner string) *events.Event { return invitation(owner, "moment") }

func invitation(owner, kind string) *events.Event {
	for _, e := range events.List(owner) {
		if e.Kind == kind {
			return e
		}
	}
	return nil
}

func scheduleCheckin(owner, clock, zone, repeat string, paused bool) error {
	return scheduleInvitation(owner, "checkin", clock, zone, repeat, paused)
}

func scheduleInvitation(owner, kind, clock, zone, repeat string, paused bool) error {
	if kind != "checkin" && kind != "moment" {
		return fmt.Errorf("unknown schedule")
	}
	if owner == "" {
		return fmt.Errorf("sign in to save this schedule")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		return fmt.Errorf("choose a valid timezone")
	}
	at, err := time.Parse("15:04", clock)
	if err != nil {
		return fmt.Errorf("choose a valid time")
	}
	if repeat != "daily" && repeat != "weekdays" && repeat != "weekly" {
		return fmt.Errorf("choose daily, weekdays or weekly")
	}
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, loc)
	for !next.After(now) || (repeat == "weekdays" && (next.Weekday() == time.Saturday || next.Weekday() == time.Sunday)) {
		next = next.AddDate(0, 0, 1)
	}
	return events.EditOwned(owner, func(records map[string]*events.Event) error {
		e := &events.Event{ID: uuid.NewString(), Owner: owner, Created: time.Now().UTC()}
		for _, old := range records {
			if old.Owner == owner && old.Kind == kind {
				*e = *old
				break
			}
		}
		e.Kind, e.Title, e.When, e.Zone, e.Repeat, e.Paused = kind, "Daily Checkin", next, zone, repeat, paused
		e.Prompt = "Ask what is on my mind. Let me set the direction; wait for my reply before taking action."
		if kind == "moment" {
			e.Title = "Take a moment"
			e.Prompt = "Offer a brief invitation to pause. No reply or task is needed."
		}
		e.Fired, e.FiredAt = false, time.Time{}
		e.Advance = scheduledAdvance(e.Kind)
		e.Sequence++
		records[e.ID] = e
		return nil
	})
}

func checkinHTML(owner, token string) string { return invitationHTML(owner, token, "checkin") }
func momentHTML(owner, token string) string  { return invitationHTML(owner, token, "moment") }

func invitationHTML(owner, token, kind string) string {
	title, description := "Daily Checkin", "How's it going? A little space to share whatever is going on, when you feel like it."
	if kind == "moment" {
		title = "Take a moment"
		description = "A gentle invitation to pause, stretch, step outside or simply rest. No reply needed."
	}
	clock, zone, repeat, status := "09:00", "", "daily", "Not scheduled"
	if kind == "moment" {
		clock = "14:00"
	}
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil {
		zone = acc.Zone
	}
	e := invitation(owner, kind)
	if e != nil {
		zone, repeat = e.Zone, e.Repeat
		loc, err := time.LoadLocation(zone)
		if err != nil {
			loc = time.UTC
		}
		clock = e.When.In(loc).Format("15:04")
		status = strings.Title(repeat) + " at " + clock + " (" + zone + ")"
		if e.Paused {
			status = "Disabled"
		}
	}
	var b strings.Builder
	b.WriteString(`<section id="` + kind + `" class="card page-stack"><h2>` + title + `</h2><p>` + description + `</p><p class="text-muted">` + html.EscapeString(status) + `</p><form method="POST" action="/agents?view=scheduled" class="form">` + app.CSRFField(token) + `<input type="hidden" name="action" value="` + kind + `-schedule"><label class="field-label">Time<input class="form-input" type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input class="form-input" name="zone" data-local-timezone required placeholder="Europe/London" value="` + html.EscapeString(zone) + `"></label><label class="field-label">Frequency<select class="form-input" name="repeat">`)
	for _, f := range []string{"daily", "weekdays", "weekly"} {
		selected := ""
		if f == repeat {
			selected = ` selected`
		}
		b.WriteString(`<option value="` + f + `"` + selected + `>` + strings.Title(f) + `</option>`)
	}
	b.WriteString(`</select></label><p class="text-sm text-muted">These reminders are included. Optional assistant replies and requested work use your usual credits. You can disable reminders at any time.</p><div class="form-actions"><button name="state" value="active">`)
	label := "Save"
	if e == nil {
		label = "Enable"
	} else if e.Paused {
		label = "Enable"
	}
	b.WriteString(label + `</button>`)
	if e != nil && !e.Paused {
		b.WriteString(`<button name="state" value="paused" class="btn-secondary">Disable</button>`)
	}
	b.WriteString(`</div></form></section>`)
	return b.String()
}

func checkinScheduleHandler(w http.ResponseWriter, r *http.Request) {
	invitationScheduleHandler(w, r, "checkin")
}
func invitationScheduleHandler(w http.ResponseWriter, r *http.Request, kind string) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Error(w, "Sign in to save this schedule", http.StatusUnauthorized)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "Reload Scheduled and try again", http.StatusForbidden)
		return
	}
	state := r.FormValue("state")
	if state != "active" && state != "paused" {
		http.Error(w, "Invalid schedule action", http.StatusBadRequest)
		return
	}
	if err := scheduleInvitation(sess.Account, kind, r.FormValue("clock"), r.FormValue("zone"), r.FormValue("repeat"), state == "paused"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/agents?view=scheduled&event="+kind, http.StatusSeeOther)
}
