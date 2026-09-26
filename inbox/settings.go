package inbox

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"strings"
	"time"
)

// SettingsHandler preserves the legacy brief API; browser setup belongs to Events.
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
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "morning"
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		var req struct {
			Period    string `json:"period"`
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
			req.Period = r.FormValue("period")
			req.Enabled, req.WorldNews, req.Zone = &enabled, &news, r.FormValue("timezone")
		} else {
			app.BadRequest(w, r, "Use the settings form or send JSON")
			return
		}
		if req.Period == "" {
			req.Period = "morning"
		}
		period = req.Period
		zone := acc.Zone
		if zone == "" || zone == "Local" {
			zone = req.Zone
		}
		if err := events.ConfigureBrief(acc.ID, *req.Enabled, *req.WorldNews, zone, req.Period); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		if !app.SendsJSON(r) && !app.WantsJSON(r) {
			http.Redirect(w, r, "/events?view=brief", http.StatusSeeOther)
			return
		}
	} else if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if period != "morning" {
		app.BadRequest(w, r, "only the morning brief is available")
		return
	}
	e := events.Brief(acc.ID, period)
	state := map[string]any{"enabled": false, "include_world_news": true, "time": "06:00", "timezone": acc.Zone}
	state["period"] = period
	if e != nil {
		state["enabled"] = !e.Paused
		state["include_world_news"] = events.BriefWorldNews(e)
		state["timezone"] = e.Zone
		at := e.When
		if loc, err := time.LoadLocation(e.Zone); err == nil {
			at = at.In(loc)
		}
		state["time"] = at.Format("15:04")
		state["repeat"] = events.BriefFrequency(acc.ID, e.Repeat)
		state["title"] = e.Title
	}
	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, state)
		return
	}
	http.Redirect(w, r, "/events?view=brief", http.StatusSeeOther)
}
