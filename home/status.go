package home

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/user"
	"net/http"
)

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	app.StatusHandler(w, r)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
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
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}
