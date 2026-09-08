package home

import (
	_ "embed"
	"encoding/json"
	"html"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/user"
)

func statusHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			http.Error(w, "Sign in again to save your status.", http.StatusUnauthorized)
		} else {
			app.RedirectToLogin(w, r)
		}
		return
	}
	if !auth.StrictCSRF(r) || !auth.CanPost(acc.ID) {
		http.Error(w, "You cannot update your status with this session.", http.StatusForbidden)
		return
	}
	if err := auth.CheckPostRate(acc.ID); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	text := r.FormValue("status")
	if r.FormValue("clear") == "1" {
		text = ""
	}
	if err := user.SetStatus(acc.ID, text); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if app.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]string{"status": user.Status(acc.ID)})
		return
	}
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

//go:embed status.js
var statusJS string

func statusForm(r *http.Request, id string) string {
	if id == "" {
		return ""
	}
	text := user.Status(id)
	label := text
	if label == "" {
		label = "What are you up to?"
	}
	return `<div id="home-status" class="page-section" data-csrf="` + html.EscapeString(auth.CSRFToken(r)) + `"><div class="form-actions inline-edit"><span class="muted">Status ·</span><button type="button" class="link-button" data-status-label aria-label="Change your public profile status">` + html.EscapeString(label) + `</button><input data-status-input hidden maxlength="160" aria-label="Your public profile status" value="` + html.EscapeString(text) + `"></div><small data-status-feedback role="status" aria-live="polite"></small></div><script>` + statusJS + `</script>`
}
