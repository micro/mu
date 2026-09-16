package inbox

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
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
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var req struct {
			Enabled   *bool  `json:"enabled"`
			WorldNews *bool  `json:"include_world_news"`
			Zone      string `json:"timezone"`
		}
		if app.DecodeJSON(r, &req) != nil || req.Enabled == nil || req.WorldNews == nil {
			app.BadRequest(w, r, "Both preferences are required")
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
	app.RespondJSON(w, state)
}
