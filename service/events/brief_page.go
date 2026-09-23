package events

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
	"strings"
	"time"
)

func briefScheduleHTML(owner string, csrf ...string) string {
	if owner == "" {
		return ""
	}
	token := ""
	if len(csrf) > 0 {
		token = csrf[0]
	}
	return briefPeriodHTML(owner, token, "morning") + briefPeriodHTML(owner, token, "evening") + `<p class="text-muted">Delivered to your Micro mail using your connected calendar, email and saved location where available. Normal usage charges apply.</p>`
}

func briefPeriodHTML(owner, token, period string) string {
	clock, zone, repeat := "06:00", "", "daily"
	if period == "evening" {
		clock = "20:00"
	}
	status := "Not scheduled"
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil {
		zone = acc.Zone
	}
	e := Brief(owner, period)
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
	title := "Morning brief"
	if period == "evening" {
		title = "Evening Debrief"
	}
	description := "Overnight developments and what matters today."
	if period == "evening" {
		description = "New developments during the day and preparation for tomorrow, without repeating unchanged news or markets."
	}
	var b strings.Builder
	b.WriteString(`<section id="` + period + `-brief" class="card page-stack"><h3>` + title + `</h3><p>` + description + `</p><div class="page-stack"><p class="text-muted">` + html.EscapeString(status) + `</p><form method="POST" action="/events" class="form">` + app.CSRFField(token) + `<input type="hidden" name="action" value="brief-schedule">`)
	selectField := func(name, title, value string, values ...string) {
		b.WriteString(`<label class="field-label">` + title + `<select class="form-input" name="` + name + `">`)
		for _, v := range values {
			selected := ""
			if v == value {
				selected = ` selected`
			}
			b.WriteString(`<option value="` + v + `"` + selected + `>` + strings.Title(v) + `</option>`)
		}
		b.WriteString(`</select></label>`)
	}
	b.WriteString(`<input type="hidden" name="period" value="` + period + `">`)
	b.WriteString(`<label class="field-label">Time<input class="form-input" type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input class="form-input" name="zone" data-local-timezone required placeholder="Europe/London" value="` + html.EscapeString(zone) + `"></label>`)
	selectField("repeat", "Frequency", repeat, "daily", "weekdays")
	checked := ""
	if BriefWorldNews(e) {
		checked = " checked"
	}
	b.WriteString(`<input type="hidden" name="news_present" value="1"><label class="check-label"><input type="checkbox" name="include_world_news" value="1"` + checked + `> Include world news</label>`)
	b.WriteString(`<div class="form-actions"><button name="state" value="active">`)
	if e == nil {
		b.WriteString("Schedule")
	} else if e.Paused {
		b.WriteString("Enable")
	} else {
		b.WriteString("Save")
	}
	b.WriteString(`</button>`)
	if e != nil && !e.Paused {
		b.WriteString(`<button name="state" value="paused" class="btn-secondary">Disable</button>`)
	}
	b.WriteString(`</div></form></div></section>`)
	return b.String()
}

func briefScheduleHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Error(w, "Sign in to schedule a brief", http.StatusUnauthorized)
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
	var news []bool
	if r.FormValue("news_present") == "1" {
		news = []bool{r.FormValue("include_world_news") == "1"}
	}
	err := scheduleBrief(sess.Account, r.FormValue("clock"), r.FormValue("zone"), r.FormValue("repeat"), r.FormValue("period"), state == "paused", false, news...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/events?view=brief", http.StatusSeeOther)
}
