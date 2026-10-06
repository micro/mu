package forms

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	sess, _, e := auth.RequireSession(r)
	if e != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Reload the page and try again.")
			return
		}
		if e := r.ParseForm(); e != nil {
			app.BadRequest(w, r, "Could not read the form.")
			return
		}
		id := r.PostForm.Get("id")
		switch r.PostForm.Get("action") {
		case "delete":
			e = Remove(owner, id)
		case "delete_response":
			e = RemoveResponse(owner, id, r.PostForm.Get("response_id"))
		case "save", "add_field":
			f := draft(r)
			if id != "" {
				if _, e = Read(owner, id); e != nil {
					app.Error(w, r, 404, "Form not found.")
					return
				}
			}
			if r.PostForm.Get("action") == "add_field" {
				if len(f.Fields) < 12 {
					f.Fields = append(f.Fields, Field{Name: nextName(f.Fields), Type: "text"})
				}
				app.Respond(w, r, app.Response{Title: "Edit form", HTML: editor(r, f, "")})
				return
			}
			var saved *Form
			saved, e = Save(owner, *f)
			if e != nil {
				app.Respond(w, r, app.Response{Title: "Edit form", HTML: editor(r, f, e.Error())})
				return
			}
			id = saved.ID
		default:
			app.BadRequest(w, r, "Unknown action.")
			return
		}
		if e != nil {
			app.BadRequest(w, r, e.Error())
			return
		}
		target := "/forms"
		if r.PostForm.Get("action") != "delete" {
			target += "?id=" + url.QueryEscape(id)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	id := r.URL.Query().Get("id")
	var body string
	title := "Forms"
	if r.URL.Query().Get("new") == "1" {
		title = "New form"
		body = editor(r, &Form{Fields: []Field{{Name: "email", Label: "Email", Type: "email"}, {Name: "message", Label: "Message", Type: "textarea", Required: true}}}, "")
	} else if id != "" {
		f, e := Read(owner, id)
		if e != nil {
			app.Error(w, r, 404, "Form not found.")
			return
		}
		title = f.Title
		if r.URL.Query().Get("edit") == "1" {
			body = editor(r, f, "")
		} else {
			body, e = view(r, owner, f)
			if e != nil {
				app.Error(w, r, 500, "Could not load responses.")
				return
			}
		}
	} else {
		fs, more, e := All(owner, offset(r))
		if e != nil {
			app.Error(w, r, 500, "Could not load forms.")
			return
		}
		body = app.CollectionControls("", app.ActionLink("/forms?new=1", "New form"), "") + `<p class="text-muted">Publish a form for anyone to fill in. Responses are only visible to you.</p><div class="collection-list">`
		for _, f := range fs {
			state := "Private"
			if f.Public {
				state = "Public"
			}
			if f.Closed {
				state += " · Closed"
			}
			body += app.CollectionItem("/forms?id="+f.ID, f.Title, f.Description, state)
		}
		body += `</div>`
		if len(fs) == 0 {
			body += `<p>No forms yet.</p>`
		}
		body += pages(r, "/forms", more)
	}
	app.Respond(w, r, app.Response{Title: title, HTML: body})
}
func offset(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if n < 0 {
		return 0
	}
	return n
}
func pages(r *http.Request, path string, more bool) string {
	if offset(r) == 0 && !more {
		return ""
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&amp;"
	}
	var b strings.Builder
	b.WriteString(`<nav class="form-actions" aria-label="Pages">`)
	if n := offset(r); n > 0 {
		prev := n - 50
		if prev < 0 {
			prev = 0
		}
		fmt.Fprintf(&b, `<a href="%s%soffset=%d">Previous</a>`, path, sep, prev)
	}
	if more {
		fmt.Fprintf(&b, `<a href="%s%soffset=%d">Next</a>`, path, sep, offset(r)+50)
	}
	b.WriteString(`</nav>`)
	return b.String()
}
func nextName(fs []Field) string {
	for i := 1; ; i++ {
		n := fmt.Sprintf("field_%d", i)
		found := false
		for _, f := range fs {
			if f.Name == n {
				found = true
			}
		}
		if !found {
			return n
		}
	}
}
func draft(r *http.Request) *Form {
	f := &Form{ID: r.PostForm.Get("id"), Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"), Public: r.PostForm.Get("public") == "on", Closed: r.PostForm.Get("closed") == "on"}
	for i := 0; i < 12; i++ {
		prefix := fmt.Sprintf("field_%d_", i)
		label := r.PostForm.Get(prefix + "label")
		if strings.TrimSpace(label) == "" {
			continue
		}
		f.Fields = append(f.Fields, Field{Name: r.PostForm.Get(prefix + "name"), Label: label, Type: r.PostForm.Get(prefix + "type"), Required: r.PostForm.Get(prefix+"required") == "on"})
	}
	return f
}
func editor(r *http.Request, f *Form, problem string) string {
	var b strings.Builder
	if problem != "" {
		b.WriteString(app.Problem(html.EscapeString(problem)))
	}
	fmt.Fprintf(&b, `<form class="form record-editor" method="POST" action="/forms">%s<input type="hidden" name="id" value="%s"><label class="field-label">Title<input name="title" maxlength="160" value="%s" required></label><label class="field-label">Description<textarea name="description" rows="3" maxlength="2000">%s</textarea></label><div class="section-body"><h2>Fields</h2><p class="text-sm text-muted">Leave a label blank to remove a field. Up to 12 fields.</p></div>`, app.CSRFField(auth.CSRFToken(r)), html.EscapeString(f.ID), html.EscapeString(f.Title), html.EscapeString(f.Description))
	for i, v := range f.Fields {
		prefix := fmt.Sprintf("field_%d_", i)
		fmt.Fprintf(&b, `<fieldset><legend>Field %d</legend><div class="form-row"><input type="hidden" name="%sname" value="%s"><label class="field-label">Label<input name="%slabel" maxlength="160" value="%s"></label><label class="field-label">Answer type<select name="%stype">`, i+1, prefix, html.EscapeString(v.Name), prefix, html.EscapeString(v.Label), prefix)
		for _, typ := range []struct{ value, label string }{{"text", "Short text"}, {"textarea", "Long text"}, {"email", "Email"}} {
			selected := ""
			if v.Type == typ.value {
				selected = " selected"
			}
			fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, typ.value, selected, typ.label)
		}
		b.WriteString(`</select></label></div>`)
		checked := ""
		if v.Required {
			checked = " checked"
		}
		fmt.Fprintf(&b, `<label class="check-label"><input type="checkbox" name="%srequired"%s> Required</label></fieldset>`, prefix, checked)
	}
	if len(f.Fields) < 12 {
		b.WriteString(`<div class="form-actions"><button type="submit" name="action" value="add_field" formnovalidate>Add field</button></div>`)
	}
	p, c := "", ""
	if f.Public {
		p = " checked"
	}
	if f.Closed {
		c = " checked"
	}
	fmt.Fprintf(&b, `<section class="section-body"><h2>Sharing</h2><label class="check-label"><input type="checkbox" name="public"%s> Public — anyone with the link can submit</label><label class="check-label"><input type="checkbox" name="closed"%s> Close submissions</label><p class="text-sm text-muted">Responses always remain private. Publishing only shares the form.</p></section><div class="form-actions"><button type="submit" name="action" value="save">Save form</button></div></form>`, p, c)
	return app.EditorPage("/forms", "Forms", b.String())
}
func view(r *http.Request, owner string, f *Form) (string, error) {
	var b strings.Builder
	b.WriteString(`<div class="page-stack"><nav class="form-actions"><a href="/forms">All forms</a><a class="btn" href="/forms?id=` + f.ID + `&amp;edit=1">Edit form</a></nav>`)
	state := "Private — only you can view this form."
	if f.Public {
		state = "Public — anyone with the link can submit."
	}
	if f.Closed {
		state += " Submissions are closed."
	}
	fmt.Fprintf(&b, `<div class="section-body"><p>%s</p><p class="text-muted">%s</p></div>`, html.EscapeString(f.Description), state)
	if f.Public {
		link := app.BaseURL(r) + "/forms/view?id=" + f.ID
		fmt.Fprintf(&b, `<section class="section-body"><div class="form-actions"><a href="%s">Open public form</a></div><label class="field-label">Share link<input readonly value="%s"></label>`, html.EscapeString(link), html.EscapeString(link))
		snippet := strings.Replace(formBody(f, nil), `action="/forms/submit`, `action="`+html.EscapeString(app.BaseURL(r))+`/forms/submit`, 1)
		fmt.Fprintf(&b, `<details class="disclosure"><summary>Use on another website</summary><p>Copy this HTML into your website. Field names must match this form. Submission limits apply: 100 per hour per form and 2,000 stored responses.</p><pre><code>%s</code></pre></details></section>`, html.EscapeString(snippet))
	}
	rs, more, e := Responses(owner, f.ID, offset(r))
	if e != nil {
		return "", e
	}
	b.WriteString(`<section class="section-body"><h2>Responses</h2><p class="text-muted">Only you can read these responses. Submitted email addresses are not verified.</p><div class="page-stack">`)
	if len(rs) == 0 {
		b.WriteString(`<p>No responses yet.</p>`)
	}
	for _, s := range rs {
		fmt.Fprintf(&b, `<article class="record-card"><p class="text-muted">%s</p><dl>`, s.Created.UTC().Format("2 Jan 2006 15:04 UTC"))
		for _, field := range s.Fields {
			fmt.Fprintf(&b, `<dt>%s</dt><dd class="preserve-whitespace">%s</dd>`, html.EscapeString(field.Label), html.EscapeString(s.Answers[field.Name]))
		}
		fmt.Fprintf(&b, `</dl><form method="POST" action="/forms" class="form-action">%s<input type="hidden" name="id" value="%s"><input type="hidden" name="response_id" value="%s"><button name="action" value="delete_response" class="btn-danger">Delete response</button></form></article>`, app.CSRFField(auth.CSRFToken(r)), f.ID, s.ID)
	}
	b.WriteString(`</div>` + pages(r, "/forms?id="+f.ID, more) + `</section>`)
	fmt.Fprintf(&b, `<details class="disclosure"><summary>Delete form</summary><p>This permanently deletes the form and all its responses.</p><form method="POST" action="/forms">%s<input type="hidden" name="id" value="%s"><button name="action" value="delete" class="btn-danger">Delete form and responses</button></form></details></div>`, app.CSRFField(auth.CSRFToken(r)), f.ID)
	return b.String(), nil
}
