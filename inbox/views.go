package inbox

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"net/url"
)

const inboxDescription = "Read and reply to your messages."

// Preserve old collection links and form submissions after moving them Home.
func collectionView(w http.ResponseWriter, r *http.Request, acc *auth.Account, view string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if view == "saved" {
		target := "/home/library"
		if kind := r.URL.Query().Get("type"); kind != "" {
			target += "?type=" + url.QueryEscape(kind)
		}
		status := http.StatusSeeOther
		if r.Method == http.MethodPost {
			status = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, target, status)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form")
			return
		}
		if r.PostForm.Get("action") != "cancel" {
			app.BadRequest(w, r, "Unknown action")
			return
		}
		if err := events.Cancel(acc.ID, r.PostForm.Get("id")); err != nil {
			app.NotFound(w, r, "Schedule not found")
			return
		}
	}
	http.Redirect(w, r, "/agents?view=scheduled", http.StatusSeeOther)
}
