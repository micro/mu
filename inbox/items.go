package inbox

import (
	"html"
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
	dest := "/inbox?kind=" + kind + "&id=" + url.QueryEscape(id)
	back := `<div class="collection-head"><a href="/inbox">← Inbox</a></div>`
	auth.SetCSRFCookie(w, r)
	csrf := auth.CSRFToken(r)
	var body string
	if kind == kindTask {
		task, err := tasks.Get(owner, id)
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
		label := agentLabel(owner, task.Agent)
		if label == "" {
			label = defaultAgentName()
		}
		body = tasks.DetailHTML(task, csrf, label, func(action string) string { return dest })
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
		body = `<div class="card record-card"><h2>` + html.EscapeString(note.Title) + `</h2>`
		body += `<p class="text-sm text-muted">Note · ` + html.EscapeString(app.TimeAgo(note.UpdatedAt)) + `</p>`
		if r.URL.Query().Get("edit") == "1" {
			body += `<form method="POST" action="` + html.EscapeString(dest) + `" class="record-editor">` + app.CSRFField(csrf) +
				`<input type="hidden" name="action" value="save"><textarea name="text" class="record-body" rows="14" maxlength="2000" required aria-label="Note">` + html.EscapeString(note.Text) +
				`</textarea><div class="form-actions"><button type="submit">Save</button><a class="btn btn-quiet" href="` + html.EscapeString(dest) + `">Cancel</a></div></form>`
		} else {
			body += `<div class="markdown-content">` + string(app.RenderLines([]byte(note.Text))) + `</div><div class="form-actions"><a class="btn btn-quiet" href="` + html.EscapeString(dest+"&edit=1") + `">Edit</a>` +
				`<form method="POST" action="` + html.EscapeString(dest) + `" onsubmit="return confirm('Delete this note?')">` + app.CSRFField(csrf) +
				`<input type="hidden" name="action" value="delete"><button class="btn btn-danger" type="submit">Delete</button></form></div>`
		}
		body += `</div>`
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: back + body})
}
