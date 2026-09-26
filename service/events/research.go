package events

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/quota"
)

func setBriefPlan(owner string, enabled bool) error {
	if enabled && auth.Plan(owner) != "pro" {
		return fmt.Errorf("a daily plan is included with Pro")
	}
	mu.Lock()
	defer mu.Unlock()
	for id, old := range events {
		if old.Owner != owner || old.Kind != "brief" || BriefPeriod(old) != "morning" {
			continue
		}
		cp := *old
		cp.Plan = enabled
		return saveFeatureLocked(id, &cp)
	}
	return nil
}

func saveFeatureLocked(id string, next *Event) error {
	list := make([]*Event, 0, len(events)+1)
	for key, e := range events {
		if key != id {
			list = append(list, e)
		}
	}
	list = append(list, next)
	if err := data.SaveJSON(storeKey, list); err != nil {
		return err
	}
	events[id] = next
	return nil
}

func Research(owner string) *Event {
	for _, e := range List(owner) {
		if e.Kind == "research" {
			return e
		}
	}
	return nil
}

func ResearchCost() int {
	return quota.OperationCost(quota.OpWebSearch) + quota.OperationCost(quota.OpAgentRun)
}

func researchHTML(owner, csrf string) string {
	if auth.Plan(owner) != "pro" {
		return `<p>Follow one topic with a private, source-linked research update on a daily or weekly schedule. Available with <a href="/pricing">Pro</a>.</p>`
	}
	topic, clock, zone, frequency, maxCredits := "", "07:00", "", "weekly", ResearchCost()
	if acc, err := auth.GetAccount(owner); err == nil {
		zone = acc.Zone
	}
	e := Research(owner)
	if e != nil {
		topic, zone, frequency, maxCredits = e.Prompt, e.Zone, e.Repeat, e.MaxCredits
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
	controls := `<button name="state" value="active">Save and enable</button>`
	if e != nil && !e.Paused {
		controls += `<button name="state" value="paused">Disable</button>`
	}
	status := "Not scheduled"
	if e != nil {
		status = "Enabled"
		if e.Paused {
			status = "Disabled"
		}
	}
	return `<div class="page-stack"><p>Follow one topic. Micro checks current web sources and sends a private update when the search results change.</p><p class="text-muted">` + status + ` · Up to ` + strconv.Itoa(ResearchCost()) + ` credits per check: one web search and one summary. Cached searches may cost less. Your limit is checked before starting.</p><form class="form" method="POST" action="/events">` + app.CSRFField(csrf) + `<input type="hidden" name="action" value="research-schedule"><label class="field-label">Topic<input name="topic" maxlength="300" required value="` + html.EscapeString(topic) + `" placeholder="What should Micro follow?"></label><label class="field-label">Frequency<select name="repeat">` + options + `</select></label><label class="field-label">Time<input type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input name="zone" data-local-timezone required value="` + html.EscapeString(zone) + `"></label><label class="field-label">Maximum credits per check<input type="number" name="max_credits" min="1" max="1000" required value="` + strconv.Itoa(maxCredits) + `"></label><div class="form-actions">` + controls + `</div></form></div>`
}

func researchScheduleHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Forbidden(w, r, "Sign in to schedule research")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reload and try again")
		return
	}
	paused := r.FormValue("state") == "paused"
	if !paused && auth.Plan(acc.ID) != "pro" {
		app.Forbidden(w, r, "Recurring research is included with Pro")
		return
	}
	if r.FormValue("state") != "active" && !paused {
		app.BadRequest(w, r, "Invalid schedule action")
		return
	}
	topic := strings.TrimSpace(r.FormValue("topic"))
	repeat := r.FormValue("repeat")
	zone := r.FormValue("zone")
	budget, e1 := strconv.Atoi(r.FormValue("max_credits"))
	clock, e2 := time.Parse("15:04", r.FormValue("clock"))
	loc, e3 := time.LoadLocation(zone)
	if topic == "" || len([]rune(topic)) > 300 || (repeat != "daily" && repeat != "weekly") || e1 != nil || budget < 1 || budget > 1000 || e2 != nil || e3 != nil || zone == "" || zone == "Local" {
		app.BadRequest(w, r, "Choose a topic, daily or weekly frequency, time, timezone and credit limit")
		return
	}
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	mu.Lock()
	defer mu.Unlock()
	var old *Event
	for _, e := range events {
		if e.Owner == acc.ID && e.Kind == "research" {
			old = e
			break
		}
	}
	e := Event{ID: uuid.NewString(), Owner: acc.ID, Kind: "research", Created: time.Now().UTC()}
	if old != nil {
		e = *old
	}
	if e.Prompt != topic {
		e.ResearchDigest, e.ResearchReport = "", ""
	}
	e.Title, e.Prompt, e.Repeat, e.Zone, e.When, e.Paused, e.MaxCredits = "Research: "+topic, topic, repeat, zone, next, paused, budget
	e.Sequence++
	e.Fired = false
	e.FiredAt = time.Time{}
	if err := saveFeatureLocked(e.ID, &e); err != nil {
		app.BadRequest(w, r, "Could not save research schedule")
		return
	}
	http.Redirect(w, r, "/events?view=research", http.StatusSeeOther)
}

// SaveResearch keeps the bounded comparison context with its owned schedule, so
// canceling the schedule or deleting the account removes both together.
func SaveResearch(source *Event, digest, report string) error {
	mu.Lock()
	defer mu.Unlock()
	old := events[source.ID]
	if old == nil || old.Owner != source.Owner || old.Kind != "research" || old.Paused || old.Sequence != source.Sequence {
		return fmt.Errorf("research schedule changed during the check")
	}
	if len(report) > 8000 {
		report = report[:8000]
	}
	cp := *old
	cp.ResearchDigest, cp.ResearchReport = digest, report
	return saveFeatureLocked(cp.ID, &cp)
}
