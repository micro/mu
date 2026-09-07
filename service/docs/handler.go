package docs

// The page: your documents, and one you are writing.
//
// It used to be a collection picker, a JSON textarea and a filter box that also
// wanted JSON — a database console. The commonest error it produced was
// "that is not valid JSON", which is a page telling you it is the wrong page.
//
// Now: a list, and an editor. Title, body, save. The body is markdown because
// markdown is what a person types when nobody makes them use a toolbar, and it
// is rendered through the untrusted path — one account's document is not
// something another account should be able to put script tags in.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /docs.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	who := sess.Account

	if r.Method == http.MethodPost && r.URL.Query().Get("import") == "1" {
		handleImport(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "could not read the form")
			return
		}
		if r.PostForm.Get("action") != "search" {
			handlePost(w, r, who)
			return
		}
		query = strings.TrimSpace(r.PostForm.Get("q"))
	}
	docs := All(who, query, 0)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"docs": docs})
		return
	}

	// One document open, or the list.
	var body string
	title := "Docs"
	switch {
	case r.URL.Query().Get("import") == "1":
		body = importPage(r, "")
		title = "Import document"
	case r.URL.Query().Get("new") == "1":
		body = editor(r, nil)
	case r.URL.Query().Get("id") != "":
		d := Get(who, r.URL.Query().Get("id"))
		if d == nil {
			body = notice("No such document.") + list(r, docs, query)
		} else if r.URL.Query().Get("edit") == "1" {
			body = editor(r, d)
		} else {
			body = view(r, d)
		}
	default:
		body = list(r, docs, query)
	}

	app.Respond(w, r, app.Response{Title: title, Description: "Your own documents", HTML: body + pageCSS})
}

// handlePost saves or deletes.
func handlePost(w http.ResponseWriter, r *http.Request, who string) {
	if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "could not read the form")
		return
	}
	if id := r.Form.Get("delete"); id != "" {
		Remove(who, id) //nolint:errcheck
		http.Redirect(w, r, "/docs", http.StatusSeeOther)
		return
	}

	doc, err := Save(who, r.Form.Get("id"), r.Form.Get("title"), r.Form.Get("content"),
		r.Form.Get("public") == "on")
	if err != nil {
		// Keep what they typed. Losing a document to a validation message is
		// worse than the mistake that caused it.
		draft := &Doc{
			ID:      r.Form.Get("id"),
			Title:   r.Form.Get("title"),
			Content: r.Form.Get("content"),
			Public:  r.Form.Get("public") == "on",
		}
		app.Respond(w, r, app.Response{Title: "Docs", Description: "Your own documents", HTML: notice(html.EscapeString(err.Error())) + editor(r, draft) + pageCSS})
		return
	}
	http.Redirect(w, r, "/docs?id="+doc.ID, http.StatusSeeOther)
}

// list is every document, newest change first.
func list(r *http.Request, docs []*Doc, query string) string {
	var b strings.Builder
	b.WriteString(`<div class="collection-head">`)
	b.WriteString(`<form method="POST" action="/docs" class="search-bar">` + app.CSRFField(auth.CSRFToken(r)) +
		`<input type="hidden" name="action" value="search">` +
		`<input type="search" name="q" value="` + html.EscapeString(query) +
		`" placeholder="Search your documents" autocomplete="off">` +
		`<button type="submit">Search</button></form>`)
	b.WriteString(`<div class="page-action"><a class="btn" href="/docs?new=1">New</a><a class="btn" href="/docs?import=1">Import</a></div>`)
	b.WriteString(`</div>`)

	if len(docs) == 0 {
		if query != "" {
			return b.String() + notice("Nothing matching "+html.EscapeString(query)+".")
		}
		return b.String() + notice("No documents yet. Anything you want to write down and come "+
			"back to — a plan, a draft, a page of notes on something. For a short thing to "+
			`remember, <a href="/notes">notes</a> is the shorter tool.`)
	}

	b.WriteString(`<div class="collection-list">`)
	for _, d := range docs {
		b.WriteString(app.CollectionItem("/docs?id="+d.ID, d.Title, snippet(d.Content), app.TimeAgo(d.Updated)))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// view is one document, read.
func view(r *http.Request, d *Doc) string {
	var b strings.Builder
	b.WriteString(`<div class="collection-head"><a class="doc-back" href="/docs">← Documents</a>`)
	b.WriteString(`<a class="doc-new" href="/docs?id=` + html.EscapeString(d.ID) + `&amp;edit=1">Edit</a></div>`)
	b.WriteString(`<article class="card record-card doc-view">`)
	b.WriteString(`<h2>` + html.EscapeString(d.Title) + `</h2>`)
	// Untrusted: this is one account's content and may be published to others.
	b.WriteString(string(app.Render([]byte(d.Content))))
	b.WriteString(`</article>`)
	fmt.Fprintf(&b, `<div class="doc-meta"><span>%s · %s</span>`, html.EscapeString(app.TimeAgo(d.Updated)),
		map[bool]string{true: "public", false: "private"}[d.Public])
	b.WriteString(`<form method="POST" action="/docs" onsubmit="return confirm('Delete this document?')">` +
		`<input type="hidden" name="delete" value="` + html.EscapeString(d.ID) + `">` +
		`<input type="hidden" name="csrf_token" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button type="submit" class="doc-delete">Delete</button></form></div>`)
	return b.String()
}

// editor is one document, being written.
func editor(r *http.Request, d *Doc) string {
	if d == nil {
		d = &Doc{}
	}
	checked := ""
	if d.Public {
		checked = " checked"
	}
	back := `<a class="doc-back" href="/docs">← Documents</a>`
	if d.ID != "" {
		back = `<a class="doc-back" href="/docs?id=` + html.EscapeString(d.ID) + `">← Back</a>`
	}
	return `<form method="POST" action="/docs" class="record-editor">
<input type="hidden" name="id" value="` + html.EscapeString(d.ID) + `">
<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">
<div class="record-controls">
<div class="record-actions">` + back + `<button type="submit">Save</button></div>
` + editorTools + `</div>
<div class="card record-card record-editor">
<input id="doc-title" class="record-title" type="text" name="title" value="` + html.EscapeString(d.Title) + `" placeholder="Title" autocomplete="off" autofocus>
<textarea id="doc-body" class="record-body" name="content" rows="24" placeholder="Write. Markdown works.">` + html.EscapeString(d.Content) + `</textarea>
</div>
<label class="doc-public"><input type="checkbox" name="public"` + checked + `> Anyone with the link can read it</label>
</form>` + editorScript
}

func notice(msg string) string {
	return `<div class="card"><p class="text-sm text-muted">` + msg + `</p></div>`
}

const pageCSS = `<style>
.doc-back{font-size:14px;color:#888;text-decoration:none}
.doc-new{font-size:14px;padding:6px 14px;border:1px solid #ccc;border-radius:6px;color:#111;text-decoration:none}
.doc-toolbar{display:flex;flex-wrap:wrap;gap:6px}
.doc-toolbar button{flex:0 0 auto;margin:0}
.doc-view{overflow-wrap:anywhere}
.doc-view h2{margin:0 0 12px}
.doc-view img{max-width:100%}
.doc-view pre{overflow-x:auto}
.doc-view table{display:block;max-width:100%;overflow-x:auto}
.doc-meta{font-size:12px;color:#888;display:flex;flex-wrap:wrap;align-items:center;gap:10px;margin:10px 2px 0}
.doc-meta form{display:inline;margin:0}
.doc-delete{background:none;border:none;color:#c00;font-size:12px;padding:0;cursor:pointer}
.doc-public{min-width:0;font-size:13px;color:#888;display:flex;align-items:center;gap:6px}
</style>`
