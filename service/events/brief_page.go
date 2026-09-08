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
	clock, zone, repeat, period := "06:00", "", "daily", "morning"
	label, status := "Manage", "Daily at 06:00 once your timezone is set"
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil {
		zone = acc.Zone
	}
	e := Brief(owner)
	if e != nil {
		zone, repeat = e.Zone, e.Repeat
		loc, err := time.LoadLocation(zone)
		if err != nil {
			loc = time.UTC
		}
		clock = e.When.In(loc).Format("15:04")
		period = "evening"
		if strings.Contains(e.Prompt, "today") {
			period = "morning"
		}
		label = "Manage"
		status = strings.Title(repeat) + " at " + clock + " (" + zone + ")"
		if e.Paused {
			status = "Disabled"
		}
	}
	title := "Morning brief"
	if period == "evening" {
		title = "Evening brief"
	}
	var b strings.Builder
	b.WriteString(`<section id="morning-brief" class="page-section"><h3>` + title + `</h3><p>Your email brief, with calendar, weather and relevant updates.</p><div class="page-stack"><p class="text-muted">` + html.EscapeString(status) + `</p><details class="disclosure"><summary>` + label + `</summary><form method="POST" action="/events" class="form">` + app.CSRFField(token) + `<input type="hidden" name="action" value="brief-schedule">`)
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
	selectField("period", "Brief", period, "evening", "morning")
	b.WriteString(`<label class="field-label">Time<input class="form-input" type="time" name="clock" required value="` + clock + `"></label><label class="field-label">Timezone<input class="form-input" name="zone" required placeholder="Europe/London" value="` + html.EscapeString(zone) + `"></label>`)
	selectField("repeat", "Frequency", repeat, "daily", "weekdays")
	b.WriteString(`<p class="text-muted">Evening looks ahead to tomorrow; morning covers today. The brief is delivered to your Mu mail, using your connected calendar, email and saved location where available. Normal usage charges apply. <a href="/account">Manage connections and email delivery</a>.</p><div class="form-actions"><button name="state" value="active">`)
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
	b.WriteString(`</div></form></details></div><script>(function(){var f=document.querySelector('form input[name="action"][value="brief-schedule"]');if(f){var z=f.form.elements.zone;if(!z.value){try{z.value=Intl.DateTimeFormat().resolvedOptions().timeZone}catch(e){}}}})();</script></section>`)
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
	err := ScheduleBrief(sess.Account, r.FormValue("clock"), r.FormValue("zone"), r.FormValue("repeat"), r.FormValue("period"), state == "paused")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/events", http.StatusSeeOther)
}
