package inbox

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
	"mu/service/tasks"
)

// itemPage composes the original records into Inbox, without copying them to threads.
func itemPage(w http.ResponseWriter, r *http.Request, owner, kind, id string) {
	w.Header().Set("Cache-Control", "private, no-store")
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
		dest = "/inbox?view=saved&kind=note&id=" + url.QueryEscape(note.ID)
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
				dest = "/inbox?view=saved&type=note"
			default:
				app.BadRequest(w, r, "Unknown action")
				return
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}
		nav := []string{}
		all := notes.All(owner)
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].UpdatedAt.Equal(all[j].UpdatedAt) {
				return all[i].ID < all[j].ID
			}
			return all[i].UpdatedAt.After(all[j].UpdatedAt)
		})
		for i, n := range all {
			if n.ID == id {
				if i > 0 {
					nav = append(nav, app.TextLink("Previous", "/inbox?view=saved&kind=note&id="+url.QueryEscape(all[i-1].ID)))
				}
				if i+1 < len(all) {
					nav = append(nav, app.TextLink("Next", "/inbox?view=saved&kind=note&id="+url.QueryEscape(all[i+1].ID)))
				}
				break
			}
		}
		body := app.Actions(app.TextLink("Saved", "/inbox?view=saved&type=note"), nav...) +
			`<div class="ib-conv page-stack"><span class="metadata-kind">Note</span><div class="ib-note-body">` + html.EscapeString(note.Text) + `</div>` +
			`<details class="disclosure"><summary>Edit</summary><form class="form" method="post">` + app.CSRFField(auth.CSRFToken(r)) +
			`<label class="field-label">Note<textarea name="text" rows="5" maxlength="2000" required>` + html.EscapeString(note.Text) + `</textarea></label><div class="form-actions"><button name="action" value="save">Save</button><button name="action" value="delete" formnovalidate>Delete</button></div></form></details></div>`
		app.Respond(w, r, app.Response{Title: note.Title, HTML: body})
		return

	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
