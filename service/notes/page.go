package notes

// The page for what you and your agents have written down.
//
// This service had no page at all. It was listed in the catalogue like
// everything else, and its tile was dead — the only place a person could read
// their own notes was a card on /account, and the one thing that read them back
// into every conversation was invisible from the service that owned them.
//
// So: a list, a delete beside each note, and a box to write one. The same three
// controls the tools give an agent, which is the point — a person and an agent
// are looking at the same notes.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
)

// Handler serves /notes.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	who := sess.Account

	if r.Method == http.MethodPost {
		handlePost(w, r, who)
		return
	}

	entries := notes.All(who)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"notes": entries})
		return
	}

	csrf := html.EscapeString(auth.CSRFToken(r))

	var b strings.Builder
	b.WriteString(`<p class="text-sm text-muted">Notes you keep, and notes your agents keep for you. ` +
		`Every one of them is read back into the questions you ask, so delete anything wrong.</p>`)

	b.WriteString(`<div class="card">`)
	if len(entries) == 0 {
		// Most accounts land here, because a note only gets written when a
		// conversation happens to contain something durable. "Nothing yet" on
		// its own reads as broken, so say what makes one appear.
		b.WriteString(`<p class="text-sm text-muted">No notes yet. Say "remember that I'm in London" ` +
			`to an agent and it will show up here, or write one below.</p>`)
	} else {
		b.WriteString(`<div class="note-list">`)
		for _, e := range entries {
			b.WriteString(`<div class="note-row"><div style="flex:1;min-width:0">` +
				`<span class="note-title">` + html.EscapeString(e.Title) + `</span>` +
				`<div class="note-text">` + html.EscapeString(e.Text) + `</div>` +
				`<div class="note-when">written ` + html.EscapeString(app.TimeAgo(e.CreatedAt)) + `</div></div>` +
				`<form method="POST" action="/notes" style="margin:0">` +
				`<input type="hidden" name="_csrf" value="` + csrf + `">` +
				`<input type="hidden" name="delete" value="` + html.EscapeString(e.Title) + `">` +
				`<button type="submit" class="link-button danger">Delete</button></form></div>`)
		}
		b.WriteString(`</div>`)
	}

	// Written as a sentence. The stored shape is a title and some text, and two
	// bare boxes of the same width say nothing about which is which; "Remember
	// that my … is …" says it without a word of explanation, and it matches the
	// sentence the empty state just told you to say to an agent.
	b.WriteString(`<form method="POST" action="/notes" class="note-add">` +
		`<input type="hidden" name="_csrf" value="` + csrf + `">` +
		`<input type="hidden" name="add" value="1">` +
		`<span class="note-word">Remember that my</span>` +
		`<input name="title" required maxlength="40" placeholder="location" class="note-in" aria-label="What the note is called">` +
		`<span class="note-word">is</span>` +
		`<input name="text" required maxlength="300" placeholder="London" class="note-in note-in-wide" aria-label="What it says">` +
		`<button type="submit">Add</button></form>`)

	if len(entries) > 0 {
		b.WriteString(`<form method="POST" action="/notes" style="margin:10px 0 0" ` +
			`onsubmit="return confirm('Delete every note?')">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="delete_all" value="1">` +
			`<button type="submit" class="link-button danger">Delete everything</button></form>`)
	}
	b.WriteString(`</div>` + pageCSS)

	w.Write([]byte(app.RenderHTMLForRequest("Notes", "What you and your agents have written down", b.String(), r)))
}

// handlePost writes, deletes one, or deletes the lot.
func handlePost(w http.ResponseWriter, r *http.Request, who string) {
	if err := r.ParseForm(); err != nil {
		app.Error(w, r, http.StatusBadRequest, "could not read that form")
		return
	}
	if !auth.ValidCSRF(r) {
		app.Error(w, r, http.StatusForbidden, "expired form, try again")
		return
	}
	switch {
	case r.Form.Get("add") != "":
		notes.Add(who, r.Form.Get("title"), r.Form.Get("text"))
	case r.Form.Get("delete") != "":
		notes.Delete(who, r.Form.Get("delete"))
	case r.Form.Get("delete_all") != "":
		notes.Clear(who)
	}
	http.Redirect(w, r, "/notes", http.StatusSeeOther)
}

const pageCSS = `<style>
.note-list{display:flex;flex-direction:column;gap:8px;margin:10px 0 0}
.note-row{display:flex;align-items:center;gap:12px;border:1px solid var(--border-color,#eee);border-radius:8px;padding:10px 14px}
.note-title{font-weight:600;font-size:14px}
.note-text{font-size:14px;margin-top:2px}
.note-when{font-size:12px;color:var(--text-muted,#999);margin-top:3px}
.note-add{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin:12px 0 0}
.note-word{font-size:14px;color:var(--text-muted,#666);white-space:nowrap}
.note-in{padding:7px 10px;border:1px solid var(--border-color,#d1d5db);border-radius:6px;font-size:14px;font-family:inherit;flex:0 1 130px;min-width:0}
.note-in-wide{flex:1 1 200px}
</style>`
