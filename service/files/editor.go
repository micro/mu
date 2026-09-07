package files

import (
	"html"
	"net/http"
	"strings"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
	store "mu/internal/files"
)

const editorLimit = 256 << 10

func editable(f *File, raw []byte) bool {
	return len(raw) <= editorLimit && utf8.Valid(raw) && !strings.ContainsRune(string(raw), 0) && (strings.HasPrefix(f.Type, "text/") || strings.Contains(f.Type, "json") || strings.Contains(f.Type, "javascript") || strings.Contains(f.Type, "xml"))
}

func editorHandler(w http.ResponseWriter, r *http.Request, id string) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	f := &File{Name: "untitled.txt", Type: "text/plain"}
	var raw []byte
	if id != "" {
		f, raw, err = Get(sess.Account, id)
		if err != nil || f.Owner != sess.Account {
			http.NotFound(w, r)
			return
		}
		if !editable(f, raw) {
			app.BadRequest(w, r, "Only UTF-8 text files up to 256 KB can be edited.")
			return
		}
	}
	message := ""
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, editorLimit*4)
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "The file is too large.")
			return
		}
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		if err := auth.CheckPostRate(sess.Account); err != nil {
			app.RespondError(w, 429, "Please wait before saving again.")
			return
		}
		text := r.PostFormValue("content")
		if id != "" && r.PostFormValue("checksum") != f.Checksum {
			f.Checksum = r.PostFormValue("checksum")
			message = "This file changed since you opened it. Your draft is below; reopen the file before replacing it."
		} else if len(text) > editorLimit || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
			message = "Use UTF-8 text up to 256 KB."
		} else {
			name := f.Name
			if id == "" {
				name = r.PostFormValue("name")
			}
			_, err = store.Replace(sess.Account, id, name, f.Type, []byte(text))
			if err == nil {
				http.Redirect(w, r, "/files", http.StatusSeeOther)
				return
			}
			message = err.Error()
		}
		raw = []byte(text)
	} else if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	readonly := ""
	if id != "" {
		readonly = " readonly"
	}
	action := "/files?new=1"
	if id != "" {
		action = "/files/" + id + "/edit"
	}
	body := `<div class="page-col"><p><a href="/files">← Files</a></p>`
	if message != "" {
		body += `<p role="alert">` + html.EscapeString(message) + `</p>`
	}
	body += `<form method="POST" action="` + html.EscapeString(action) + `" class="card">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="checksum" value="` + html.EscapeString(f.Checksum) + `"><label>File name<input class="field field-wide" name="name" required maxlength="255" value="` + html.EscapeString(f.Name) + `"` + readonly + `></label><label>Contents<textarea class="field field-wide" name="content" rows="22">` + html.EscapeString(string(raw)) + `</textarea></label><button type="submit">Save file</button></form></div>`
	app.Respond(w, r, app.Response{Title: "Files", HTML: body})
}
