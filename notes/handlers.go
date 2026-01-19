package notes

import (
	"html"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
)

// Handler handles /notes routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/notes")
	path = strings.TrimPrefix(path, "/")

	// Require auth for all notes operations
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	switch {
	case path == "" || path == "/":
		handleList(w, r, sess, acc)
	case path == "new":
		handleNew(w, r, sess)
	case strings.HasSuffix(path, "/delete"):
		id := strings.TrimSuffix(path, "/delete")
		handleDelete(w, r, sess, id)
	case strings.HasSuffix(path, "/archive"):
		id := strings.TrimSuffix(path, "/archive")
		handleArchive(w, r, sess, id)
	case strings.HasSuffix(path, "/pin"):
		id := strings.TrimSuffix(path, "/pin")
		handlePin(w, r, sess, id)
	default:
		handleView(w, r, sess, path)
	}
}

func handleList(w http.ResponseWriter, r *http.Request, sess *auth.Session, acc *auth.Account) {
	showArchived := r.URL.Query().Get("archived") == "true"
	tagFilter := r.URL.Query().Get("tag")
	searchQuery := r.URL.Query().Get("q")

	var notesList []*Note
	if searchQuery != "" {
		notesList = SearchNotes(sess.Account, searchQuery, 0)
	} else {
		notesList = ListNotes(sess.Account, showArchived, tagFilter, 0)
	}

	// JSON response
	if app.WantsJSON(r) {
		app.RespondJSON(w, notesList)
		return
	}

	// Get all tags for filter
	allTags := GetAllTags(sess.Account)

	// Build HTML
	var content strings.Builder
	content.WriteString(notesCSS)
	content.WriteString(`<div class="notes-container">`)

	// Header with search and new button
	content.WriteString(`<div class="notes-header">`)
	content.WriteString(`<a href="/notes/new" class="new-note-btn">+ New Note</a>`)
	content.WriteString(`<form class="notes-search" action="/notes" method="GET">`)
	content.WriteString(`<input type="text" name="q" placeholder="Search notes..." value="` + html.EscapeString(searchQuery) + `">`)
	content.WriteString(`</form>`)
	content.WriteString(`</div>`)

	// Tags filter
	if len(allTags) > 0 {
		content.WriteString(`<div class="tags-filter">`)
		for _, tag := range allTags {
			active := ""
			if tag == tagFilter {
				active = " active"
			}
			content.WriteString(`<a href="/notes?tag=` + tag + `" class="tag` + active + `">` + html.EscapeString(tag) + `</a>`)
		}
		if tagFilter != "" {
			content.WriteString(`<a href="/notes" class="tag clear">Clear</a>`)
		}
		content.WriteString(`</div>`)
	}

	// Archive toggle
	content.WriteString(`<div class="view-toggle">`)
	if showArchived {
		content.WriteString(`<a href="/notes">Notes</a> · <strong>Archive</strong>`)
	} else {
		content.WriteString(`<strong>Notes</strong> · <a href="/notes?archived=true">Archive</a>`)
	}
	content.WriteString(`</div>`)

	// Notes grid
	if len(notesList) == 0 {
		if searchQuery != "" {
			content.WriteString(`<p class="empty">No notes found for "` + html.EscapeString(searchQuery) + `"</p>`)
		} else if showArchived {
			content.WriteString(`<p class="empty">No archived notes</p>`)
		} else {
			content.WriteString(`<p class="empty">No notes yet. Create your first note!</p>`)
		}
	} else {
		content.WriteString(`<div class="notes-grid">`)
		for _, n := range notesList {
			content.WriteString(renderNoteCard(n))
		}
		content.WriteString(`</div>`)
	}

	content.WriteString(`</div>`)

	w.Write([]byte(app.RenderHTML("Notes", "Your notes", content.String())))
}

func renderNoteCard(n *Note) string {
	var b strings.Builder

	colorClass := ""
	if n.Color != "" {
		colorClass = " color-" + n.Color
	}

	b.WriteString(`<div class="note-card` + colorClass + `">`)

	// Pin indicator
	if n.Pinned {
		b.WriteString(`<span class="pin-icon" title="Pinned">📌</span>`)
	}

	// Title
	if n.Title != "" {
		b.WriteString(`<h4><a href="/notes/` + n.ID + `">` + html.EscapeString(n.Title) + `</a></h4>`)
	}

	// Content preview
	content := n.Content
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	b.WriteString(`<a href="/notes/` + n.ID + `" class="note-content">` + html.EscapeString(content) + `</a>`)

	// Tags
	if len(n.Tags) > 0 {
		b.WriteString(`<div class="note-tags">`)
		for _, tag := range n.Tags {
			b.WriteString(`<span class="tag">` + html.EscapeString(tag) + `</span>`)
		}
		b.WriteString(`</div>`)
	}

	// Footer with time
	b.WriteString(`<div class="note-meta">` + app.TimeAgo(n.UpdatedAt) + `</div>`)

	b.WriteString(`</div>`)
	return b.String()
}

func handleNew(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	if r.Method == "POST" {
		r.ParseForm()
		title := strings.TrimSpace(r.FormValue("title"))
		content := strings.TrimSpace(r.FormValue("content"))
		tagsStr := r.FormValue("tags")

		if content == "" {
			renderNewForm(w, "Content is required", title, content, tagsStr)
			return
		}

		tags := parseTags(tagsStr)
		note, err := CreateNote(sess.Account, title, content, tags)
		if err != nil {
			renderNewForm(w, err.Error(), title, content, tagsStr)
			return
		}

		http.Redirect(w, r, "/notes/"+note.ID, 302)
		return
	}

	renderNewForm(w, "", "", "", "")
}

func renderNewForm(w http.ResponseWriter, errMsg, title, content, tags string) {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<div class="error">` + html.EscapeString(errMsg) + `</div>`
	}

	formHTML := notesCSS + errHTML + `
<form method="POST" class="note-editor">
  <input type="text" name="title" placeholder="Title" value="` + html.EscapeString(title) + `">
  <textarea name="content" placeholder="Take a note..." required autofocus>` + html.EscapeString(content) + `</textarea>
  <input type="text" name="tags" placeholder="Tags (comma-separated)" value="` + html.EscapeString(tags) + `">
  <div class="note-actions">
    <button type="submit">Save</button>
    <a href="/notes">Cancel</a>
  </div>
</form>`

	w.Write([]byte(app.RenderHTML("New Note", "", formHTML)))
}

func handleView(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	note := GetNote(id, sess.Account)
	if note == nil {
		app.NotFound(w, r, "Note not found")
		return
	}

	// Handle edit form submission
	if r.Method == "POST" {
		r.ParseForm()
		title := strings.TrimSpace(r.FormValue("title"))
		content := strings.TrimSpace(r.FormValue("content"))
		tagsStr := r.FormValue("tags")
		pinned := r.FormValue("pinned") == "on"
		color := r.FormValue("color")

		if content == "" {
			renderViewForm(w, note, "Content is required")
			return
		}

		tags := parseTags(tagsStr)
		err := UpdateNote(id, sess.Account, title, content, tags, pinned, note.Archived, color)
		if err != nil {
			renderViewForm(w, note, err.Error())
			return
		}

		http.Redirect(w, r, "/notes/"+id, 302)
		return
	}

	// JSON response
	if app.WantsJSON(r) {
		app.RespondJSON(w, note)
		return
	}

	renderViewForm(w, note, "")
}

func renderViewForm(w http.ResponseWriter, n *Note, errMsg string) {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<div class="error">` + html.EscapeString(errMsg) + `</div>`
	}

	pinnedChecked := ""
	if n.Pinned {
		pinnedChecked = " checked"
	}

	tagsStr := strings.Join(n.Tags, ", ")

	colorOptions := ""
	colors := []string{"", "yellow", "green", "blue", "pink", "purple", "gray"}
	for _, c := range colors {
		selected := ""
		if c == n.Color {
			selected = " selected"
		}
		label := "Default"
		if c != "" {
			label = strings.Title(c)
		}
		colorOptions += `<option value="` + c + `"` + selected + `>` + label + `</option>`
	}

	formHTML := notesCSS + errHTML + `
<form method="POST" class="note-editor">
  <input type="text" name="title" placeholder="Title" value="` + html.EscapeString(n.Title) + `">
  <textarea name="content" placeholder="Take a note..." required>` + html.EscapeString(n.Content) + `</textarea>
  <details class="note-options-toggle">
    <summary>Options</summary>
    <div class="note-options">
      <label><input type="checkbox" name="pinned"` + pinnedChecked + `> Pinned</label>
      <label>Color: <select name="color">` + colorOptions + `</select></label>
      <input type="text" name="tags" placeholder="Tags (comma-separated)" value="` + html.EscapeString(tagsStr) + `">
    </div>
  </details>
  <div class="note-actions">
    <button type="submit">Save</button>
    <a href="/notes">Back</a>
    <a href="/notes/` + n.ID + `/archive">` + archiveLabel(n.Archived) + `</a>
    <a href="/notes/` + n.ID + `/delete" class="delete-link" onclick="return confirm('Delete this note?')">Delete</a>
  </div>
</form>
<div class="note-meta-info">` + app.TimeAgo(n.UpdatedAt) + `</div>`

	title := "Note"
	if n.Title != "" {
		title = n.Title
	}
	w.Write([]byte(app.RenderHTML(title, "", formHTML)))
}

func archiveLabel(archived bool) string {
	if archived {
		return "Unarchive"
	}
	return "Archive"
}

func handleDelete(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	if err := DeleteNote(id, sess.Account); err != nil {
		app.NotFound(w, r, "Note not found")
		return
	}
	http.Redirect(w, r, "/notes", 302)
}

func handleArchive(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	note := GetNote(id, sess.Account)
	if note == nil {
		app.NotFound(w, r, "Note not found")
		return
	}

	// Toggle archive status
	if err := ArchiveNote(id, sess.Account, !note.Archived); err != nil {
		app.ServerError(w, r, err.Error())
		return
	}

	http.Redirect(w, r, "/notes", 302)
}

func handlePin(w http.ResponseWriter, r *http.Request, sess *auth.Session, id string) {
	note := GetNote(id, sess.Account)
	if note == nil {
		app.NotFound(w, r, "Note not found")
		return
	}

	// Toggle pin status
	if err := PinNote(id, sess.Account, !note.Pinned); err != nil {
		app.ServerError(w, r, err.Error())
		return
	}

	http.Redirect(w, r, "/notes", 302)
}

const notesCSS = `
<style>
.notes-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; gap: 15px; flex-wrap: wrap; }
.new-note-btn { padding: 8px 12px; background: var(--btn-primary, #000); color: white !important; text-decoration: none; border-radius: var(--border-radius, 6px); display: inline-block; font-size: 14px; }
.new-note-btn:hover { background: var(--btn-primary-hover, #333); }
.notes-search input { padding: 10px 15px; border: 1px solid #ddd; border-radius: 6px; min-width: 200px; }
.tags-filter { margin-bottom: 15px; display: flex; gap: 8px; flex-wrap: wrap; }
.tags-filter .tag { padding: 4px 12px; background: #f0f0f0; border-radius: 20px; text-decoration: none; color: #333; font-size: 13px; }
.tags-filter .tag.active { background: var(--accent-color, #0d7377); color: white; }
.tags-filter .tag.clear { background: #ddd; }
.view-toggle { margin-bottom: 20px; font-size: 14px; color: #666; }
.view-toggle a { color: #666; text-decoration: none; }
.view-toggle a:hover { text-decoration: underline; }
.notes-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 16px; }
.note-card { background: #fff; border: 1px solid #e8e8e8; border-radius: 8px; padding: 16px; position: relative; transition: box-shadow 0.15s; }
.note-card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
.note-card h4 { margin: 0 0 8px 0; }
.note-card h4 a { text-decoration: none; color: #1a1a1a; }
.note-card .note-content { display: block; color: #555; font-size: 14px; line-height: 1.5; white-space: pre-wrap; text-decoration: none; margin-bottom: 10px; }
.note-card .note-tags { margin-top: 10px; }
.note-card .note-tags .tag { display: inline-block; padding: 2px 8px; background: #f0f0f0; border-radius: 10px; font-size: 11px; color: #666; margin-right: 4px; }
.note-card .note-meta { font-size: 12px; color: #999; margin-top: 10px; }
.note-card .pin-icon { position: absolute; top: 10px; right: 10px; font-size: 14px; }
.note-card.color-yellow { background: #fff9c4; border-color: #fff176; }
.note-card.color-green { background: #c8e6c9; border-color: #a5d6a7; }
.note-card.color-blue { background: #bbdefb; border-color: #90caf9; }
.note-card.color-pink { background: #f8bbd9; border-color: #f48fb1; }
.note-card.color-purple { background: #e1bee7; border-color: #ce93d8; }
.note-card.color-gray { background: #f5f5f5; border-color: #e0e0e0; }
.empty { color: #888; text-align: center; padding: 40px; }
.note-editor { max-width: 600px; }
.note-editor input[type="text"] { width: 100%; padding: 8px 0; border: none; border-bottom: 1px solid #eee; font-size: 18px; font-weight: 500; margin-bottom: 8px; outline: none; }
.note-editor input[type="text"]:focus { border-bottom-color: var(--accent-color, #0d7377); }
.note-editor input[type="text"]::placeholder { color: #aaa; font-weight: normal; }
.note-editor textarea { width: 100%; min-height: 200px; padding: 8px 0; border: none; font-size: 15px; font-family: inherit; line-height: 1.6; resize: none; outline: none; }
.note-editor textarea::placeholder { color: #aaa; }
.note-options-toggle { margin: 16px 0; }
.note-options-toggle summary { font-size: 13px; color: #666; cursor: pointer; }
.note-options { padding-top: 12px; display: flex; flex-direction: column; gap: 10px; }
.note-options label { display: inline-flex; align-items: center; gap: 6px; font-size: 14px; color: #555; }
.note-options input[type="checkbox"] { width: auto; margin: 0; }
.note-options select { padding: 6px 10px; border: 1px solid #ddd; border-radius: 4px; }
.note-options input[type="text"] { padding: 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; }
.note-actions { display: flex; gap: 15px; align-items: center; padding-top: 16px; border-top: 1px solid #eee; margin-top: 16px; }
.note-actions button { padding: 10px 24px; background: var(--accent-color, #0d7377); color: white; border: none; border-radius: 6px; cursor: pointer; }
.note-actions a { color: #666; text-decoration: none; font-size: 14px; }
.note-actions .delete-link { color: #c00; }
.note-meta-info { margin-top: 16px; font-size: 13px; color: #999; }
.error { color: #c00; padding: 10px; background: #fee; border-radius: 6px; margin-bottom: 15px; }
</style>
`
