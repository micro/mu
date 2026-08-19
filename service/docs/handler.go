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

	if r.Method == http.MethodPost {
		handlePost(w, r, who)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	docs := All(who, query, 0)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"docs": docs})
		return
	}

	// One document open, or the list.
	var body string
	switch {
	case r.URL.Query().Get("new") == "1":
		body = editor(r, nil)
	case r.URL.Query().Get("id") != "":
		d := Get(who, r.URL.Query().Get("id"))
		if d == nil {
			body = notice("No such document.") + list(docs, query)
		} else if r.URL.Query().Get("edit") == "1" {
			body = editor(r, d)
		} else {
			body = view(r, d)
		}
	default:
		body = list(docs, query)
	}

	app.Respond(w, r, app.Response{Title: "Docs", Description: "Your own documents", HTML: body + pageCSS})
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
func list(docs []*Doc, query string) string {
	var b strings.Builder
	b.WriteString(`<div class="doc-head">`)
	b.WriteString(`<form method="GET" action="/docs" class="doc-search">` +
		`<input type="text" name="q" value="` + html.EscapeString(query) +
		`" placeholder="Search your documents" autocomplete="off">` +
		`<button type="submit">Search</button></form>`)
	b.WriteString(`<a class="doc-new" href="/docs?new=1">New document</a>`)
	b.WriteString(`</div>`)

	if len(docs) == 0 {
		if query != "" {
			return b.String() + notice("Nothing matching "+html.EscapeString(query)+".")
		}
		return b.String() + notice("No documents yet. Anything you want to write down and come "+
			"back to — a plan, a draft, a page of notes on something. For a short thing to "+
			`remember, <a href="/notes">notes</a> is the shorter tool.`)
	}

	b.WriteString(`<div class="doc-list">`)
	for _, d := range docs {
		b.WriteString(`<a class="doc-row" href="/docs?id=` + html.EscapeString(d.ID) + `">`)
		b.WriteString(`<span class="doc-title">` + html.EscapeString(d.Title) + `</span>`)
		if s := snippet(d.Content); s != "" {
			b.WriteString(`<span class="doc-snip">` + html.EscapeString(s) + `</span>`)
		}
		b.WriteString(`<span class="doc-when">` + html.EscapeString(app.TimeAgo(d.Updated)) + `</span>`)
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// view is one document, read.
func view(r *http.Request, d *Doc) string {
	var b strings.Builder
	b.WriteString(`<div class="doc-head"><a class="doc-back" href="/docs">← Documents</a>`)
	b.WriteString(`<a class="doc-new" href="/docs?id=` + html.EscapeString(d.ID) + `&amp;edit=1">Edit</a></div>`)
	b.WriteString(`<article class="card doc-view">`)
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
	return `<div class="doc-head">` + back + `</div>
<form method="POST" action="/docs" class="card doc-editor">
<input type="hidden" name="id" value="` + html.EscapeString(d.ID) + `">
<input type="hidden" name="csrf_token" value="` + html.EscapeString(auth.CSRFToken(r)) + `">
<input class="doc-title-input" type="text" name="title" value="` + html.EscapeString(d.Title) + `" placeholder="Title" autocomplete="off" autofocus>
<textarea class="doc-body" name="content" rows="24" placeholder="Write. Markdown works.">` + html.EscapeString(d.Content) + `</textarea>
<div class="doc-actions">
<label class="doc-public"><input type="checkbox" name="public"` + checked + `> Anyone with the link can read it</label>
<button type="submit">Save</button>
</div>
</form>`
}

func notice(msg string) string {
	return `<div class="card"><p class="text-sm text-muted">` + msg + `</p></div>`
}

const pageCSS = `<style>
.doc-head{display:flex;align-items:center;gap:12px;margin:0 0 16px;flex-wrap:wrap}
.doc-search{display:flex;gap:8px;flex:1;min-width:220px}
.doc-search input{flex:1;min-width:0}
.doc-new{margin-left:auto;font-size:14px;padding:6px 14px;border:1px solid #ccc;border-radius:6px;color:#111;text-decoration:none;white-space:nowrap}
.doc-back{font-size:14px;color:#888;text-decoration:none}
.doc-list{border:1px solid #eee;border-radius:8px;overflow:hidden}
.doc-row{display:flex;align-items:baseline;gap:10px;padding:12px 14px;border-bottom:1px solid #f4f4f4;text-decoration:none;color:inherit}
.doc-row:last-child{border-bottom:none}
.doc-row:hover{background:#fafafa}
.doc-title{font-weight:600;font-size:14px;white-space:nowrap}
.doc-snip{color:#888;font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}
.doc-when{color:#bbb;font-size:12px;margin-left:auto;white-space:nowrap}
.doc-view h2{margin:0 0 12px}
.doc-view img{max-width:100%}
.doc-meta{font-size:12px;color:#888;display:flex;align-items:center;gap:10px;margin:10px 2px 0}
.doc-meta form{display:inline;margin:0}
.doc-delete{background:none;border:none;color:#c00;font-size:12px;padding:0;cursor:pointer}
.doc-editor{display:flex;flex-direction:column;gap:10px}
.doc-title-input{font-size:1.15rem;font-weight:600;border:none;border-bottom:1px solid #eee;padding:6px 0;outline:none}
.doc-body{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;line-height:1.6;border:1px solid #eee;border-radius:6px;padding:12px;resize:vertical}
.doc-actions{display:flex;align-items:center;gap:12px}
.doc-public{font-size:13px;color:#888;display:flex;align-items:center;gap:6px}
.doc-actions button{margin-left:auto}
</style>`
