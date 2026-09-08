package home

import (
	"html"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/user"
)

func statusHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !auth.ValidCSRF(r) || !auth.CanPost(acc.ID) {
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
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func statusForm(r *http.Request, id string) string {
	if id == "" {
		return ""
	}
	return `<details class="disclosure page-section"><summary>Your status</summary>` +
		`<form class="form" method="post" action="/home">` +
		`<input type="hidden" name="action" value="status">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<label class="field-label">Shown on your profile<input class="field field-wide" name="status" maxlength="160" placeholder="What are you up to?" value="` + html.EscapeString(user.Status(id)) + `"></label>` +
		`<div class="form-actions"><button type="submit">Save</button><button type="submit" name="clear" value="1">Clear</button></div></form></details>`
}
