package notes

// The page for what you and your agents have written down.
//
// This service had no page at all. Its tile in the catalogue was dead and
// /cache was a 404, while the only place a person could read their own notes
// was a card on /account — the wrong home for the thing an agent writes to.
//
// The first version of this page was that card moved across: a row per note and
// a one-line form reading "Remember that my … is …". That is a memory widget,
// not notes. A note has a body you write into, you open one to read it, and you
// change it and save. So: a list you click into, an editor with a textarea, and
// a New note button — the same three things every notes app has ever had.

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
)

// maxText is as long as one note may be.
//
// Notes are read back into the system prompt of every question the owner asks,
// so their total length is a real cost paid on every turn — fifty notes of
// unbounded size is a prompt nobody can afford. Long enough to write in, short
// enough that fifty of them still fit.
const maxText = 2000

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

	q := r.URL.Query()
	if title := strings.TrimSpace(q.Get("note")); title != "" {
		if text := notes.Get(who, title); text != "" {
			render(w, r, "Notes", editor(r, title, text))
			return
		}
		// A note that is not there any more — deleted in another tab, or a
		// guessed URL. The list is a better answer than an error.
		http.Redirect(w, r, "/notes", http.StatusSeeOther)
		return
	}
	if q.Get("new") != "" {
		render(w, r, "New note", editor(r, "", ""))
		return
	}

	render(w, r, "Notes", list(entries))
}

// list is every note, newest change first, each one a link into the editor.
func list(entries []*notes.Entry) string {
	var b strings.Builder
	b.WriteString(`<div class="note-head">` +
		`<p class="text-sm text-muted m-0">Notes you keep, and notes your agents keep ` +
		`for you. All of them are read back into the questions you ask.</p>` +
		`<a class="note-new" href="/notes?new=1">New note</a></div>`)

	if len(entries) == 0 {
		// Most accounts land here, because an agent only writes a note when a
		// conversation happens to contain something durable. "Nothing yet" on
		// its own reads as broken, so say what makes one appear.
		b.WriteString(`<div class="card"><p class="text-sm text-muted">No notes yet. Say ` +
			`"remember that I'm in London" to an agent and it will show up here, or write one ` +
			`yourself.</p></div>` + pageCSS)
		return b.String()
	}

	// Newest change first: the note you touched last is the one you are most
	// likely to want again, and the store keeps them in the order they were
	// first written.
	ordered := make([]*notes.Entry, len(entries))
	copy(ordered, entries)
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}

	b.WriteString(`<div class="note-grid">`)
	for _, e := range ordered {
		when := e.UpdatedAt
		if when.IsZero() {
			when = e.CreatedAt
		}
		b.WriteString(`<a class="note-card" href="/notes?note=` + html.EscapeString(urlArg(e.Title)) + `">` +
			`<span class="note-card-title">` + html.EscapeString(e.Title) + `</span>` +
			`<span class="note-card-body">` + html.EscapeString(preview(e.Text)) + `</span>` +
			`<span class="note-card-when">` + html.EscapeString(app.TimeAgo(when)) + `</span></a>`)
	}
	b.WriteString(`</div>` + pageCSS)
	return b.String()
}

// editor writes one note. An empty title means a new one.
func editor(r *http.Request, title, text string) string {
	csrf := html.EscapeString(auth.CSRFToken(r))

	titleField := `<input name="title" class="note-title-in" required maxlength="40" ` +
		`placeholder="Title" autofocus value="` + html.EscapeString(title) + `">`
	if title != "" {
		// The title is the note's address — rewriting it would leave the old
		// note behind and make a second one. Renaming means delete and write,
		// and it is not worth a control that looks like an edit and is not.
		titleField = `<input class="note-title-in" value="` + html.EscapeString(title) +
			`" readonly aria-label="Title">` +
			`<input type="hidden" name="title" value="` + html.EscapeString(title) + `">`
	}

	var b strings.Builder
	b.WriteString(`<div class="note-head">` +
		`<a class="link" href="/notes">← All notes</a></div>`)
	b.WriteString(`<div class="card"><form method="POST" action="/notes" class="note-editor">` +
		`<input type="hidden" name="_csrf" value="` + csrf + `">` +
		`<input type="hidden" name="save" value="1">` +
		titleField +
		`<textarea name="text" class="note-body-in" rows="14" required maxlength="` +
		strconv.Itoa(maxText) + `" placeholder="Write it down">` + html.EscapeString(text) + `</textarea>` +
		`<div class="note-actions"><button type="submit">Save</button></div></form>`)

	if title != "" {
		b.WriteString(`<form method="POST" action="/notes" class="note-delete" ` +
			`onsubmit="return confirm('Delete this note?')">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="delete" value="` + html.EscapeString(title) + `">` +
			`<button type="submit" class="link-button danger">Delete</button></form>`)
	}
	b.WriteString(`</div>` + pageCSS)
	return b.String()
}

// handlePost saves one note or deletes one.
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
	case r.Form.Get("save") != "":
		title := strings.TrimSpace(r.Form.Get("title"))
		text := strings.TrimSpace(r.Form.Get("text"))
		if len(text) > maxText {
			text = text[:maxText]
		}
		notes.Add(who, title, text)
		http.Redirect(w, r, "/notes?note="+urlArg(title), http.StatusSeeOther)
		return
	case r.Form.Get("delete") != "":
		notes.Delete(who, r.Form.Get("delete"))
	}
	http.Redirect(w, r, "/notes", http.StatusSeeOther)
}

// render wraps a body in the page shell.
func render(w http.ResponseWriter, r *http.Request, title, body string) {
	app.Respond(w, r, app.Response{Title: title, Description: "What you and your agents have written down", HTML: body})
}

// preview is the first line or so of a note, for the card that opens it.
func preview(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(text) <= 120 {
		return text
	}
	cut := text[:120]
	if i := strings.LastIndex(cut, " "); i > 60 {
		cut = cut[:i]
	}
	return cut + "…"
}

func urlArg(s string) string { return url.QueryEscape(s) }

const pageCSS = `<style>
.note-head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin:0 0 14px}
.note-new,.note-new:visited{display:inline-block;background:#111;color:#fff;text-decoration:none;padding:7px 14px;
  border-radius:8px;font-size:13px;font-weight:600;white-space:nowrap}
.note-new:hover,.note-new:visited:hover{background:#333;color:#fff}
.note-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:10px}
.note-card{display:flex;flex-direction:column;gap:5px;padding:14px;border:1px solid var(--border-color,#e5e5e5);
  border-radius:10px;text-decoration:none;min-height:110px}
.note-card:hover{border-color:#bbb}
.note-card-title{font-size:14px;font-weight:600;color:var(--text-color,#111)}
.note-card-body{font-size:13px;color:var(--text-muted,#666);line-height:1.45;flex:1}
.note-card-when{font-size:12px;color:var(--text-muted,#999)}
.note-editor{display:flex;flex-direction:column;gap:10px}
.note-title-in{padding:9px 12px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;
  font-size:15px;font-weight:600;font-family:inherit;width:100%}
.note-title-in[readonly]{background:transparent;border-color:transparent;padding-left:0;font-size:19px}
.note-body-in{padding:11px 12px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;
  font-size:14px;font-family:inherit;line-height:1.55;resize:vertical;width:100%}
.note-actions{display:flex;gap:10px;align-items:center}
.note-delete{margin:12px 0 0}
</style>`
