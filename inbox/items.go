package inbox

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
	"mu/service/tasks"
)

// itemPage composes the original records into Inbox, without copying them to threads.
func itemPage(w http.ResponseWriter, r *http.Request, owner, kind, id string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if r.Method == http.MethodPost && !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	dest := "/work?id=" + url.QueryEscape(id)

	auth.SetCSRFCookie(w, r)

	if kind == kindTask {
		_, err := tasks.Get(owner, id)
		if err != nil {
			app.NotFound(w, r, "Task not found")
			return
		}
		if r.Method == http.MethodPost {
			action := r.FormValue("action")
			if err := tasks.ApplyAction(owner, id, action); err != nil {
				app.BadRequest(w, r, err.Error())
				return
			}
			if action == "delete" {
				dest = "/inbox"
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}

	} else {
		var note *notes.Entry
		for _, n := range notes.All(owner) {
			if n.ID == id {
				note = n
				break
			}
		}
		if note == nil {
			app.NotFound(w, r, "Note not found")
			return
		}
		dest = "/notes?id=" + url.QueryEscape(note.ID)
		if r.Method == http.MethodPost {
			switch r.FormValue("action") {
			case "save":
				text := strings.TrimSpace(r.FormValue("text"))
				if text == "" || len(text) > 2000 {
					app.BadRequest(w, r, "Write a note of up to 2000 bytes")
					return
				}
				notes.AddFrom(owner, note.Title, text, note.SourceThread)
			case "delete":
				notes.Delete(owner, note.Title)
				dest = "/inbox"
			default:
				app.BadRequest(w, r, "Unknown action")
				return
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
