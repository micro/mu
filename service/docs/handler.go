package docs

// The page exists so the service list can be honest.
//
// A capability with no page is absent from the catalogue, the sidebar and every
// count, and db was the only account-scoped service in that position — which is
// how somebody could be told to store things here and never find a way to look
// at what they had stored. It is also the more useful half of the argument: a
// database an agent writes to and nobody can read is indistinguishable from one
// that is silently dropping writes.
//
// It was a viewer only, on the argument that records are written by an agent or
// an app and a form here would be a fourth way to do the same thing. That was
// wrong twice over. Two doors onto one service is the shape of the whole
// product — an agent calls the tool, a person opens the page — so a form here is
// the second door, not a fourth way. And a store you cannot fix by hand is one
// you cannot use: when an agent writes the wrong thing, the owner's only remedy
// was to write an agent to undo it.
//
// So the four verbs the tools have, the page has: create, read, list, delete.
// Nothing here does anything docs_* cannot, and the query controls are docs_list's
// own arguments under their own names — the page shows you the call it just
// made, so browsing your data teaches the tool that reads it.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

// Handler serves /docs — the caller's collections, and one collection's records.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		handlePost(w, r, owner)
		return
	}

	if name := strings.TrimSpace(r.URL.Query().Get("collection")); name != "" {
		renderCollection(w, r, owner, name)
		return
	}

	cols, err := userdb.Collections(namespace, owner)
	if err != nil {
		app.Error(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"collections": cols})
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<p class="card-desc">Named collections of records, private unless you publish them. ` +
		`This is what your agents store through <code>docs_create</code> — apps keep their own ` +
		`separate store under <code>mu.db</code>.</p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<div class="notice"><p>` + html.EscapeString(msg) + `</p></div>`)
	}

	if len(cols) == 0 {
		b.WriteString(`<p class="text-muted">Nothing stored yet. A collection is made the first time ` +
			`something writes to it — there is no schema to declare.</p>`)
	} else {
		b.WriteString(`<div class="docs-cols">`)
		for _, c := range cols {
			b.WriteString(`<a class="docs-col" href="/docs?collection=` + html.EscapeString(c.Name) + `">`)
			b.WriteString(`<span class="docs-col-name">` + html.EscapeString(c.Name) + `</span>`)
			b.WriteString(`<span class="docs-col-meta">` + plural(c.Records, "record") +
				` · ` + html.EscapeString(app.TimeAgo(c.Updated)) + `</span>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}

	// A collection is made by its first record, so "new collection" and "new
	// record" are the same form with the name typed in rather than chosen.
	b.WriteString(`<details class="docs-new"` + openIf(len(cols) == 0) + `>`)
	b.WriteString(`<summary>New collection</summary>`)
	b.WriteString(recordForm(csrf, "", "", nil, false))
	b.WriteString(`</details>`)

	b.WriteString(dbCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Database", "Your own records", b.String(), r)))
}

// handlePost is the write half: store a record, or delete one.
//
// Both go through userdb with the session's account as the owner, which is the
// same call and the same ownership check the tools make. The page cannot reach
// anything an agent holding your token could not.
func handlePost(w http.ResponseWriter, r *http.Request, owner string) {
	collection := strings.TrimSpace(r.FormValue("collection"))
	back := "/docs"
	if collection != "" {
		back = "/docs?collection=" + urlArg(collection)
	}
	fail := func(msg string) {
		http.Redirect(w, r, back+joiner(back)+"error="+urlArg(msg), http.StatusSeeOther)
	}

	switch r.FormValue("action") {
	case "delete":
		if err := userdb.Delete(namespace, owner, collection, r.FormValue("id")); err != nil {
			fail(err.Error())
			return
		}
	case "save":
		if collection == "" {
			fail("name the collection")
			return
		}
		var data map[string]any
		body := strings.TrimSpace(r.FormValue("data"))
		if body == "" {
			fail("a record needs some data")
			return
		}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			// The most common mistake by a distance, and json's own message for
			// it ("invalid character 'x' looking for beginning of object key
			// string") does not say it.
			fail("that is not valid JSON: " + err.Error())
			return
		}
		public := r.FormValue("public") != ""
		if id := strings.TrimSpace(r.FormValue("id")); id != "" {
			if _, err := userdb.Update(namespace, owner, collection, id, data, public); err != nil {
				fail(err.Error())
				return
			}
		} else if _, err := userdb.Create(namespace, owner, collection, data, public); err != nil {
			fail(err.Error())
			return
		}
	default:
		fail("unknown action")
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// query is docs_list's arguments, read off the URL. Same names, same defaults, so
// the controls on the page and the tool an agent calls are one thing.
type query struct {
	Scope string
	Where map[string]any
	Sort  string
	Order string
	Limit int
	Raw   string // the where box as typed, so a bad filter survives the round trip
	Err   string
}

func queryFrom(r *http.Request) query {
	q := query{
		Scope: r.URL.Query().Get("scope"),
		Sort:  strings.TrimSpace(r.URL.Query().Get("sort")),
		Order: r.URL.Query().Get("order"),
		Raw:   strings.TrimSpace(r.URL.Query().Get("where")),
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		q.Limit = n
	}
	if q.Raw != "" {
		if err := json.Unmarshal([]byte(q.Raw), &q.Where); err != nil {
			// Show every record and say why the filter was ignored, rather than
			// showing nothing and letting it read as an empty collection.
			q.Where, q.Err = nil, "filter ignored — not valid JSON: "+err.Error()
		}
	}
	return q
}

func renderCollection(w http.ResponseWriter, r *http.Request, owner, name string) {
	q := queryFrom(r)
	recs, err := userdb.List(namespace, owner, name, q.Scope, q.Where, q.Sort, q.Order, q.Limit)
	if err != nil {
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"collection": name, "records": recs})
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<p><a class="link" href="/docs">← All collections</a></p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<div class="notice"><p>` + html.EscapeString(msg) + `</p></div>`)
	}
	if q.Err != "" {
		b.WriteString(`<div class="notice"><p>` + html.EscapeString(q.Err) + `</p></div>`)
	}

	b.WriteString(queryForm(name, q))
	b.WriteString(`<p class="card-desc">` + plural(len(recs), "record") + ` in <strong>` +
		html.EscapeString(name) + `</strong> — the same question as ` +
		`<code>` + html.EscapeString(listCall(name, q)) + `</code></p>`)

	// Open when there is nothing to look at. A collapsed form on an empty page
	// is the whole complaint that got this written: the page said the store was
	// empty and offered no way to put anything in it.
	b.WriteString(`<details class="docs-new"` + openIf(len(recs) == 0 && q.Where == nil) + `>`)
	b.WriteString(`<summary>New record</summary>`)
	b.WriteString(recordForm(csrf, name, "", nil, false))
	b.WriteString(`</details>`)

	if len(recs) == 0 {
		if q.Where != nil {
			b.WriteString(`<p class="text-muted">Nothing matches that filter.</p>`)
		} else {
			b.WriteString(`<p class="text-muted">This collection is empty.</p>`)
		}
	}
	for i := range recs {
		rec := recs[i]
		pretty, _ := json.MarshalIndent(rec.Data, "", "  ")
		vis := "private"
		if rec.Public {
			vis = "public"
		}
		b.WriteString(`<div class="docs-rec">`)
		b.WriteString(`<div class="docs-rec-meta"><code>` + html.EscapeString(rec.ID) + `</code> · ` +
			vis + ` · ` + html.EscapeString(app.TimeAgo(rec.Updated)))
		// Delete is a form, not a link: it changes something, and a link that
		// changes something is followed by every crawler and prefetcher there is.
		b.WriteString(`<form method="POST" action="/docs" class="docs-del" onsubmit="return confirm('Delete this record?')">` +
			`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
			`<input type="hidden" name="action" value="delete">` +
			`<input type="hidden" name="collection" value="` + html.EscapeString(name) + `">` +
			`<input type="hidden" name="id" value="` + html.EscapeString(rec.ID) + `">` +
			`<button type="submit" class="docs-act docs-remove">Delete</button></form>`)
		b.WriteString(`</div>`)
		b.WriteString(`<details class="docs-edit"><summary class="docs-act">Edit</summary>`)
		b.WriteString(recordForm(csrf, name, rec.ID, pretty, rec.Public))
		b.WriteString(`</details>`)
		b.WriteString(`<pre class="docs-code">` + html.EscapeString(string(pretty)) + `</pre>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(dbCSS)
	w.Write([]byte(app.RenderHTMLForRequest(name, "Records in "+name, b.String(), r)))
}

// recordForm writes and rewrites a record. Given an id it overwrites that one,
// which is exactly what docs_create does with an id — one code path, one meaning.
func recordForm(csrf, collection, id string, data []byte, public bool) string {
	var b strings.Builder
	b.WriteString(`<form method="POST" action="/docs" class="docs-form">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">`)
	b.WriteString(`<input type="hidden" name="action" value="save">`)
	if id != "" {
		b.WriteString(`<input type="hidden" name="id" value="` + html.EscapeString(id) + `">`)
	}
	if collection == "" {
		b.WriteString(`<label class="docs-label">Collection</label>`)
		b.WriteString(`<input name="collection" required maxlength="60" placeholder="notes" class="docs-input">`)
	} else {
		b.WriteString(`<input type="hidden" name="collection" value="` + html.EscapeString(collection) + `">`)
	}
	body := `{"text": "remember this"}`
	if len(data) > 0 {
		body = string(data)
	}
	b.WriteString(`<label class="docs-label">Data <span class="docs-hint">— a JSON object; the fields are yours to choose</span></label>`)
	b.WriteString(`<textarea name="data" rows="6" required class="docs-input docs-mono">` +
		html.EscapeString(body) + `</textarea>`)
	checked := ""
	if public {
		checked = " checked"
	}
	b.WriteString(`<label class="docs-check"><input type="checkbox" name="public" value="1"` + checked +
		`> Public <span class="docs-hint">— readable by anyone with the id</span></label>`)
	b.WriteString(`<button type="submit" class="docs-save">Save record</button>`)
	b.WriteString(`</form>`)
	return b.String()
}

// queryForm is docs_list's arguments as controls. A GET form, so the query lives
// in the URL and a useful view can be bookmarked or sent to somebody.
func queryForm(collection string, q query) string {
	sel := func(name, val string, opts [][2]string) string {
		var s strings.Builder
		s.WriteString(`<select name="` + name + `" class="docs-input docs-narrow">`)
		for _, o := range opts {
			on := ""
			if o[0] == val {
				on = " selected"
			}
			s.WriteString(`<option value="` + o[0] + `"` + on + `>` + o[1] + `</option>`)
		}
		s.WriteString(`</select>`)
		return s.String()
	}
	limit := ""
	if q.Limit > 0 {
		limit = strconv.Itoa(q.Limit)
	}
	var b strings.Builder
	b.WriteString(`<form method="GET" action="/docs" class="docs-query">`)
	b.WriteString(`<input type="hidden" name="collection" value="` + html.EscapeString(collection) + `">`)
	b.WriteString(`<input name="where" class="docs-input docs-mono" placeholder='where, e.g. {"done": false}' value="` +
		html.EscapeString(q.Raw) + `">`)
	b.WriteString(`<div class="docs-query-row">`)
	b.WriteString(sel("scope", q.Scope, [][2]string{{"", "Mine"}, {"public", "Public"}, {"all", "All"}}))
	b.WriteString(`<input name="sort" class="docs-input docs-narrow" placeholder="sort by field" value="` +
		html.EscapeString(q.Sort) + `">`)
	b.WriteString(sel("order", q.Order, [][2]string{{"", "Newest"}, {"asc", "Ascending"}}))
	b.WriteString(`<input name="limit" type="number" min="1" max="200" class="docs-input docs-narrow" placeholder="limit" value="` +
		html.EscapeString(limit) + `">`)
	b.WriteString(`<button type="submit" class="docs-run">Run</button>`)
	if q.Raw != "" || q.Sort != "" || q.Scope != "" || q.Limit > 0 {
		b.WriteString(`<a class="link" href="/docs?collection=` + urlArg(collection) + `">Clear</a>`)
	}
	b.WriteString(`</div></form>`)
	return b.String()
}

// listCall renders the query as the docs_list call that would ask the same thing,
// so the page teaches the tool rather than being an alternative to it.
func listCall(collection string, q query) string {
	args := map[string]any{"collection": collection}
	if q.Where != nil {
		args["where"] = q.Where
	}
	if q.Scope != "" {
		args["scope"] = q.Scope
	}
	if q.Sort != "" {
		args["sort"] = q.Sort
	}
	if q.Order != "" {
		args["order"] = q.Order
	}
	if q.Limit > 0 {
		args["limit"] = q.Limit
	}
	// Marshalled through an ordered slice: map order is random in Go, and a
	// snippet that reshuffles itself on every reload reads as a bug.
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v, _ := json.Marshal(args[k])
		parts = append(parts, fmt.Sprintf("%q: %s", k, v))
	}
	return "docs_list {" + strings.Join(parts, ", ") + "}"
}

func openIf(b bool) string {
	if b {
		return " open"
	}
	return ""
}

func joiner(u string) string {
	if strings.Contains(u, "?") {
		return "&"
	}
	return "?"
}

// urlArg escapes a value for a query string. Kept local rather than reaching for
// url.QueryEscape on a whole URL, because these are single values.
func urlArg(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

const dbCSS = `<style>
.docs-cols{display:flex;flex-direction:column;gap:8px;margin-top:12px}
.docs-col{display:flex;justify-content:space-between;align-items:baseline;gap:12px;
  padding:12px 14px;border:1px solid #eee;border-radius:8px;text-decoration:none;color:inherit}
.docs-col:hover{border-color:#bbb}
.docs-col-name{font-weight:600;font-size:14px}
.docs-col-meta{font-size:12px;color:#999}
.docs-rec{margin-bottom:16px}
.docs-rec-meta{display:flex;align-items:center;gap:8px;font-size:12px;color:#999;margin-bottom:4px}
.docs-rec-meta code{font-size:11px}
.docs-code{background:#f7f7f7;border:1px solid #eee;border-radius:6px;padding:10px 12px;
  font-size:12px;overflow-x:auto;margin:0}
.docs-form{max-width:640px;margin:10px 0 4px}
.docs-label{display:block;font-size:13px;font-weight:600;color:#374151;margin:10px 0 5px}
.docs-hint{font-weight:400;color:#9ca3af}
.docs-input{width:100%;box-sizing:border-box;padding:8px 10px;font-size:14px;
  border:1px solid #d1d5db;border-radius:6px;font-family:inherit}
.docs-mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}
.docs-narrow{width:auto;flex:0 1 150px}
/* A checkbox next to text never lines up on its own baseline; the row owns the
   alignment so the label and the box agree about where the middle is. */
.docs-check{display:flex;align-items:center;gap:7px;font-size:13px;color:#444;margin:10px 0}
.docs-check input{width:auto;flex:none;margin:0}
.docs-save,.docs-run{padding:8px 18px;font-size:14px;font-weight:600;border:0;
  border-radius:var(--border-radius,6px);background:var(--btn-primary,#000);color:#fff;cursor:pointer}
.docs-save{margin-top:4px}
.docs-query{max-width:640px;margin:14px 0 6px}
.docs-query-row{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin-top:8px}
.docs-new{max-width:640px;margin:16px 0}
.docs-new>summary,.docs-edit>summary{cursor:pointer;font-size:13px;color:#666}
.docs-new>summary:hover,.docs-edit>summary:hover{color:#111}
.docs-edit{margin:0 0 6px}
.docs-del{display:inline;margin:0}
.docs-act{background:none;border:0;color:#666;font-size:12px;cursor:pointer;padding:0}
.docs-act:hover{color:#111;text-decoration:underline}
.docs-remove:hover{color:#b00}
</style>`
