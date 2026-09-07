package home

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
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
	clock, zone, repeat, period := "20:00", "", "daily", "evening"
	label, status := "Schedule", ""
	if acc, err := auth.GetAccount(owner); err == nil && acc != nil {
		zone = acc.Zone
	}
	e := events.Brief(owner)
	if e != nil {
		zone, repeat = e.Zone, e.Repeat
		loc, err := time.LoadLocation(zone)
		if err != nil {
			loc = time.UTC
		}
		clock = e.When.In(loc).Format("15:04")
		if strings.Contains(e.Prompt, "today") {
			period = "morning"
		}
		label = "Manage"
		status = strings.Title(repeat) + " at " + clock + " (" + zone + ")"
		if e.Paused {
			status = "Paused"
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="mt-3"><span class="text-muted">` + html.EscapeString(status) + `</span><details><summary>` + label + `</summary><form method="POST" action="/home" class="col gap-2 mt-3">` + app.CSRFField(token) + `<input type="hidden" name="action" value="brief-schedule">`)
	selectField := func(name, title, value string, values ...string) {
		b.WriteString(`<label>` + title + `<select class="form-input" name="` + name + `">`)
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
	b.WriteString(`<label>Time<input class="form-input" type="time" name="clock" required value="` + clock + `"></label><label>Timezone<input class="form-input" name="zone" required placeholder="Europe/London" value="` + html.EscapeString(zone) + `"></label>`)
	selectField("repeat", "Frequency", repeat, "daily", "weekdays")
	b.WriteString(`<p class="text-muted">Evening looks ahead to tomorrow; morning covers today. The brief is delivered to your Mu mail, using your connected calendar, email and saved location where available. Normal usage charges apply. <a href="/account">Manage connections and email delivery</a>.</p><div class="d-flex gap-2 flex-wrap"><button name="state" value="active">`)
	if e == nil {
		b.WriteString("Schedule")
	} else if e.Paused {
		b.WriteString("Resume")
	} else {
		b.WriteString("Save")
	}
	b.WriteString(`</button>`)
	if e != nil && !e.Paused {
		b.WriteString(`<button name="state" value="paused" class="btn-secondary">Pause</button>`)
	}
	b.WriteString(`</div></form></details></div><script>(function(){var f=document.querySelector('form input[name="action"][value="brief-schedule"]');if(f){var z=f.form.elements.zone;if(!z.value){try{z.value=Intl.DateTimeFormat().resolvedOptions().timeZone}catch(e){}}}})();</script>`)
	return b.String()
}

func briefScheduleHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Error(w, "Sign in to schedule a brief", http.StatusUnauthorized)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "Reload Home and try again", http.StatusForbidden)
		return
	}
	state := r.FormValue("state")
	if state != "active" && state != "paused" {
		http.Error(w, "Invalid schedule action", http.StatusBadRequest)
		return
	}
	err := events.ScheduleBrief(sess.Account, r.FormValue("clock"), r.FormValue("zone"), r.FormValue("repeat"), r.FormValue("period"), state == "paused")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}
