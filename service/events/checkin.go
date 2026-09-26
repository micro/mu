package events

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"mu/internal/app"
	"mu/internal/auth"
)

// Checkin returns the owner's optional check-in, independently of their brief.
func Checkin(owner string) *Event {
	for _, e := range List(owner) {
		if e.Kind == "checkin" {
			return e
		}
	}
	return nil
}

func scheduleCheckin(owner, clock, zone, repeat string, paused bool) error {
	if owner == "" {
		return fmt.Errorf("sign in to schedule a check-in")
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
	mu.Lock()
	defer mu.Unlock()
	e := &Event{ID: uuid.NewString(), Owner: owner, Created: time.Now().UTC()}
	for _, old := range events {
		if old.Owner == owner && old.Kind == "checkin" {
			*e = *old
			break
		}
	}
	e.Kind, e.Title, e.When, e.Zone, e.Repeat, e.Paused = "checkin", "Daily Checkin", next, zone, repeat, paused
	e.Prompt = "Ask what I want to focus on today. Wait for my reply before planning or taking action."
	e.Fired, e.FiredAt = false, time.Time{}
	e.Sequence++
	return saveFeatureLocked(e.ID, e)
}

func checkinHTML(owner, token string) string {
	clock, zone, repeat, status := "09:00", "", "daily", "Not scheduled"
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil {
		zone = acc.Zone
	}
	e := Checkin(owner)
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
	b.WriteString(`<section id="checkin" class="card page-stack"><h3>Daily Checkin</h3><p>A separate conversation when you are ready to start your day. Micro shows upcoming commitments and outstanding tasks, then asks what you want to focus on. Reply in one or two sentences about what you need to get done or need help with. No follow-up nudges.</p><p class="text-muted">` + html.EscapeString(status) + `</p><form method="POST" action="/events" class="form">` + app.CSRFField(token) + `<input type="hidden" name="action" value="checkin-schedule"><label class="field-label">Time<input class="form-input" type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input class="form-input" name="zone" data-local-timezone required placeholder="Europe/London" value="` + html.EscapeString(zone) + `"></label><label class="field-label">Frequency<select class="form-input" name="repeat">`)
	for _, f := range []string{"daily", "weekdays", "weekly"} {
		selected := ""
		if f == repeat {
			selected = ` selected`
		}
		b.WriteString(`<option value="` + f + `"` + selected + `>` + strings.Title(f) + `</option>`)
	}
	b.WriteString(`</select></label><p class="text-sm text-muted">The check-in is included. Assistant replies and any work you request use your usual credits. Nothing is changed until you ask.</p><div class="form-actions"><button name="state" value="active">`)
	label := "Save"
	if e == nil {
		label = "Enable check-in"
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
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Error(w, "Sign in to schedule a check-in", http.StatusUnauthorized)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "Reload Events and try again", http.StatusForbidden)
		return
	}
	state := r.FormValue("state")
	if state != "active" && state != "paused" {
		http.Error(w, "Invalid schedule action", http.StatusBadRequest)
		return
	}
	if err := scheduleCheckin(sess.Account, r.FormValue("clock"), r.FormValue("zone"), r.FormValue("repeat"), state == "paused"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/events?view=brief#checkin", http.StatusSeeOther)
}
