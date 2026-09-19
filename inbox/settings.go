package inbox

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"strings"
	"time"
)

// SettingsHandler controls only the caller's built-in delivery preferences.
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			app.Unauthorized(w, r)
		} else {
			app.RedirectToLogin(w, r)
		}
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		var req struct {
			Enabled   *bool  `json:"enabled"`
			WorldNews *bool  `json:"include_world_news"`
			Zone      string `json:"timezone"`
		}
		if app.SendsJSON(r) {
			if app.DecodeJSON(r, &req) != nil || req.Enabled == nil || req.WorldNews == nil {
				app.BadRequest(w, r, "Both preferences are required")
				return
			}
		} else if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			enabled, news := r.FormValue("enabled") == "1", r.FormValue("include_world_news") == "1"
			req.Enabled, req.WorldNews, req.Zone = &enabled, &news, r.FormValue("timezone")
		} else {
			app.BadRequest(w, r, "Use the settings form or send JSON")
			return
		}
		zone := acc.Zone
		if zone == "" || zone == "Local" {
			zone = req.Zone
		}
		if err := events.ConfigureBrief(acc.ID, *req.Enabled, *req.WorldNews, zone); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		if !app.SendsJSON(r) && !app.WantsJSON(r) {
			http.Redirect(w, r, "/inbox/settings", http.StatusSeeOther)
			return
		}
	} else if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	e := events.Brief(acc.ID)
	effectiveDefault := !acc.Agent && !acc.Unclaimed && !acc.Banned && acc.Zone != "" && acc.Zone != "Local"
	if _, err := time.LoadLocation(acc.Zone); err != nil {
		effectiveDefault = false
	}
	state := map[string]any{"enabled": effectiveDefault, "include_world_news": true, "time": "06:00", "timezone": acc.Zone}
	if e != nil {
		state["enabled"] = !e.Paused
		state["include_world_news"] = events.BriefWorldNews(e)
		state["timezone"] = e.Zone
		at := e.When
		if loc, err := time.LoadLocation(e.Zone); err == nil {
			at = at.In(loc)
		}
		state["time"] = at.Format("15:04")
		state["repeat"] = e.Repeat
		state["title"] = e.Title
	}
	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, state)
		return
	}
	checked := func(value any) string {
		if value == true {
			return " checked"
		}
		return ""
	}
	zone := fmt.Sprint(state["timezone"])
	body := viewNavigation("settings") + `<h2>Morning brief</h2><form class="form" method="post" action="/inbox/settings">` + app.CSRFField(auth.CSRFToken(r)) +
		`<label class="check-label"><input type="checkbox" name="enabled" value="1"` + checked(state["enabled"]) + `> Morning brief</label>` +
		`<p class="note">A brief at ` + html.EscapeString(fmt.Sprint(state["time"])) + `. Reply to its email or open its notification to continue the conversation.</p>` +
		`<label class="check-label"><input type="checkbox" name="include_world_news" value="1"` + checked(state["include_world_news"]) + `> Include world news</label>`
	if zone == "" || zone == "Local" {
		body += app.Field{Name: "timezone", Label: "Timezone", Placeholder: "Europe/London", Required: true}.HTML()
	} else {
		body += `<p class="note">Timezone: ` + html.EscapeString(zone) + `</p>`
	}
	body += `<div class="form-actions"><button type="submit">Save</button><a href="/inbox">Inbox</a></div></form>`
	body += app.Section("Mail clients", `<p>Read and reply to your conversations in your own mail app.</p><div class="form-actions"><a href="/account/clients">Client setup</a><a href="/inbox/imap">Mail settings</a></div>`)
	app.Respond(w, r, app.Response{Title: "Inbox settings", HTML: body})
}
