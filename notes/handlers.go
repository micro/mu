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

	// Build filters HTML
	var filters strings.Builder
	
	// Tags filter
	allTags := GetAllTags(sess.Account)
	if len(allTags) > 0 {
		for _, tag := range allTags {
			active := ""
			if tag == tagFilter {
				active = " active"
			}
			filters.WriteString(`<a href="/notes?tag=` + tag + `" class="tag` + active + `">` + html.EscapeString(tag) + `</a>`)
		}
		if tagFilter != "" {
			filters.WriteString(`<a href="/notes" class="tag clear">Clear</a>`)
		}
	}
	
	// Archive toggle
	filters.WriteString(`<span class="view-toggle">`)
	if showArchived {
		filters.WriteString(`<a href="/notes">Notes</a> · <strong>Archive</strong>`)
	} else {
		filters.WriteString(`<strong>Notes</strong> · <a href="/notes?archived=true">Archive</a>`)
	}
	filters.WriteString(`</span>`)

	// Build content
	var content strings.Builder
	for _, n := range notesList {
		content.WriteString(renderNoteCard(n))
	}

	// Empty message
	emptyMsg := ""
	if len(notesList) == 0 {
		if searchQuery != "" {
			emptyMsg = `No notes found for "` + searchQuery + `"`
		} else if showArchived {
			emptyMsg = "No archived notes"
		} else {
			emptyMsg = "No notes yet. Create your first note!"
		}
	}

	// Use app.Page for consistent layout
	gridContent := ""
	if content.Len() > 0 {
		gridContent = app.Grid(content.String())
	}

	pageHTML := app.Page(app.PageOpts{
		Action:  "/notes/new",
		Label:   "+ New Note",
		Search:  "/notes",
		Query:   searchQuery,
		Filters: filters.String(),
		Content: gridContent,
		Empty:   emptyMsg,
	})

	w.Write([]byte(app.RenderHTML("Notes", "Your notes", pageHTML)))
}

func renderNoteCard(n *Note) string {
	var b strings.Builder

	colorClass := ""
	if n.Color != "" {
		colorClass = " card-" + n.Color
	}

	b.WriteString(`<div class="card card-note` + colorClass + `">`)

	// Pin indicator
	if n.Pinned {
		b.WriteString(`<span class="card-pin" title="Pinned">📌</span>`)
	}

	// Title
	if n.Title != "" {
		b.WriteString(app.Title(n.Title, "/notes/"+n.ID))
	}

	// Content preview
	content := n.Content
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	b.WriteString(`<a href="/notes/` + n.ID + `" class="card-content">` + html.EscapeString(content) + `</a>`)

	// Tags
	b.WriteString(app.Tags(n.Tags, ""))

	// Footer with time
	b.WriteString(`<div class="card-meta">` + app.TimeAgo(n.UpdatedAt) + `</div>`)

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

	formHTML := errHTML + `
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

	formHTML := errHTML + `
<form method="POST" class="note-editor" data-note-id="` + n.ID + `">
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
    <span id="autosave-status"></span>
    <a href="/notes">Back</a>
    <a href="/notes/` + n.ID + `/archive">` + archiveLabel(n.Archived) + `</a>
    <a href="/notes/` + n.ID + `/delete" class="delete-link" onclick="return confirm('Delete this note?')">Delete</a>
  </div>
</form>
<div class="note-meta-info">` + app.TimeAgo(n.UpdatedAt) + `</div>
<script>
(function() {
  const form = document.querySelector('.note-editor[data-note-id]');
  if (!form) return;
  
  const noteId = form.dataset.noteId;
  const status = document.getElementById('autosave-status');
  const storageKey = 'note_original_' + noteId;
  let saveTimeout = null;
  let lastSaved = {};
  let original = null;
  
  function getFormData() {
    return {
      title: form.querySelector('[name=title]').value,
      content: form.querySelector('[name=content]').value,
      tags: form.querySelector('[name=tags]').value,
      pinned: form.querySelector('[name=pinned]').checked,
      color: form.querySelector('[name=color]').value
    };
  }
  
  function setFormData(data) {
    form.querySelector('[name=title]').value = data.title || '';
    form.querySelector('[name=content]').value = data.content || '';
    form.querySelector('[name=tags]').value = data.tags || '';
    form.querySelector('[name=pinned]').checked = data.pinned || false;
    form.querySelector('[name=color]').value = data.color || '';
  }
  
  function hasChanges() {
    const current = getFormData();
    return JSON.stringify(current) !== JSON.stringify(lastSaved);
  }
  
  function showRevert() {
    if (document.getElementById('revert-link')) return;
    const link = document.createElement('a');
    link.id = 'revert-link';
    link.href = '#';
    link.textContent = 'Undo';
    link.className = 'revert-link';
    link.onclick = function(e) {
      e.preventDefault();
      revertToOriginal();
    };
    status.parentNode.insertBefore(link, status.nextSibling);
  }
  
  function hideRevert() {
    const link = document.getElementById('revert-link');
    if (link) link.remove();
  }
  
  function revertToOriginal() {
    if (!original) return;
    setFormData(original);
    lastSaved = getFormData();
    autoSave();
    hideRevert();
    status.textContent = 'Reverted';
    status.className = 'autosave-saved';
    setTimeout(() => { status.textContent = ''; }, 2000);
  }
  
  function autoSave() {
    if (!hasChanges()) return;
    
    const data = getFormData();
    status.textContent = 'Saving...';
    status.className = 'autosave-saving';
    
    const formData = new FormData();
    formData.append('title', data.title);
    formData.append('content', data.content);
    formData.append('tags', data.tags);
    if (data.pinned) formData.append('pinned', 'on');
    formData.append('color', data.color);
    
    fetch('/notes/' + noteId, {
      method: 'POST',
      body: formData
    }).then(r => {
      if (r.ok || r.redirected) {
        lastSaved = data;
        status.textContent = 'Saved';
        status.className = 'autosave-saved';
        showRevert();
        setTimeout(() => { status.textContent = ''; }, 2000);
      } else {
        status.textContent = 'Save failed';
        status.className = 'autosave-error';
      }
    }).catch(() => {
      status.textContent = 'Save failed';
      status.className = 'autosave-error';
    });
  }
  
  function scheduleAutoSave() {
    if (saveTimeout) clearTimeout(saveTimeout);
    saveTimeout = setTimeout(autoSave, 1500);
  }
  
  // Store original state on first load
  const stored = localStorage.getItem(storageKey);
  if (stored) {
    original = JSON.parse(stored);
  } else {
    original = getFormData();
    localStorage.setItem(storageKey, JSON.stringify(original));
  }
  lastSaved = getFormData();
  
  // Clear stored original when navigating away
  window.addEventListener('beforeunload', () => {
    localStorage.removeItem(storageKey);
  });
  
  // Listen for changes
  form.querySelectorAll('input, textarea, select').forEach(el => {
    el.addEventListener('input', scheduleAutoSave);
    el.addEventListener('change', scheduleAutoSave);
  });
  
  // Prevent form submission (auto-save handles it)
  form.addEventListener('submit', e => {
    e.preventDefault();
    autoSave();
  });
})();
</script>`

	// Use generic title - the editable input field shows the note title
	w.Write([]byte(app.RenderHTML("Note", "", formHTML)))
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

// Note editor styles are in mu.css
