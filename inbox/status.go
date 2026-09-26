package inbox

import (
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
	http.Redirect(w, r, "/@"+acc.ID, http.StatusSeeOther)
}

// A normal compact form works without page-local scripts or save-on-blur.
func statusForm(r *http.Request, id string) string {
	if id == "" {
		return ""
	}
	return `<form id="profile-status" method="post" action="/@` + html.EscapeString(id) + `" class="page-stack"><input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `"><input type="hidden" name="action" value="status"><label for="profile-status-text" class="text-sm text-muted">Public status</label><div class="form-actions"><input id="profile-status-text" name="status" maxlength="160" placeholder="A short update others can see" value="` + html.EscapeString(user.Status(id)) + `"><button type="submit">Save</button><button type="submit" name="clear" value="1">Clear</button></div></form>`
}
