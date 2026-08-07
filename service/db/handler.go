package db

// The page exists so the service list can be honest.
//
// A capability with no page is absent from the catalogue, the sidebar and every
// count, and db was the only account-scoped service in that position — which is
// how somebody could be told to store things here and never find a way to look
// at what they had stored. It is also the more useful half of the argument: a
// database an agent writes to and nobody can read is indistinguishable from one
// that is silently dropping writes.
//
// It is a viewer, not an editor. Records are written by an agent or an app, and
// a form here would be a fourth way to do the same thing.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

// Handler serves /db — the caller's collections, and one collection's records.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	owner := sess.Account

	if name := strings.TrimSpace(r.URL.Query().Get("collection")); name != "" {
		renderCollection(w, r, owner, name)
		return
	}

	cols, err := userdb.Collections(namespace, owner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"collections": cols})
		return
	}

	var b strings.Builder
	b.WriteString(`<p class="card-desc">Named collections of records, private unless you publish them. ` +
		`This is what your agents store through <code>db_create</code> — apps keep their own ` +
		`separate store under <code>mu.db</code>.</p>`)

	if len(cols) == 0 {
		b.WriteString(`<p class="text-muted">Nothing stored yet. A collection is made the first time ` +
			`something writes to it — there is no schema to declare:</p>` +
			`<pre class="db-code">db_create {"collection": "notes", "data": {"text": "remember this"}}</pre>`)
	} else {
		b.WriteString(`<div class="db-cols">`)
		for _, c := range cols {
			b.WriteString(`<a class="db-col" href="/db?collection=` + html.EscapeString(c.Name) + `">`)
			b.WriteString(`<span class="db-col-name">` + html.EscapeString(c.Name) + `</span>`)
			b.WriteString(`<span class="db-col-meta">` + plural(c.Records, "record") +
				` · ` + html.EscapeString(app.TimeAgo(c.Updated)) + `</span>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(dbCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Database", "Your own records", b.String(), r)))
}

func renderCollection(w http.ResponseWriter, r *http.Request, owner, name string) {
	recs, err := userdb.List(namespace, owner, name, "mine", nil, "", "", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"collection": name, "records": recs})
		return
	}

	var b strings.Builder
	b.WriteString(`<p><a class="link" href="/db">← All collections</a></p>`)
	b.WriteString(`<p class="card-desc">` + plural(len(recs), "record") + ` in <strong>` +
		html.EscapeString(name) + `</strong>.</p>`)

	if len(recs) == 0 {
		b.WriteString(`<p class="text-muted">This collection is empty.</p>`)
	}
	for _, rec := range recs {
		pretty, _ := json.MarshalIndent(rec.Data, "", "  ")
		vis := "private"
		if rec.Public {
			vis = "public"
		}
		b.WriteString(`<div class="db-rec">`)
		b.WriteString(`<div class="db-rec-meta"><code>` + html.EscapeString(rec.ID) + `</code> · ` +
			vis + ` · ` + html.EscapeString(app.TimeAgo(rec.Updated)) + `</div>`)
		b.WriteString(`<pre class="db-code">` + html.EscapeString(string(pretty)) + `</pre>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(dbCSS)
	w.Write([]byte(app.RenderHTMLForRequest(name, "Records in "+name, b.String(), r)))
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

const dbCSS = `<style>
.db-cols{display:flex;flex-direction:column;gap:8px;margin-top:12px}
.db-col{display:flex;justify-content:space-between;align-items:baseline;gap:12px;
  padding:12px 14px;border:1px solid #eee;border-radius:8px;text-decoration:none;color:inherit}
.db-col:hover{border-color:#bbb}
.db-col-name{font-weight:600;font-size:14px}
.db-col-meta{font-size:12px;color:#999}
.db-rec{margin-bottom:12px}
.db-rec-meta{font-size:12px;color:#999;margin-bottom:4px}
.db-rec-meta code{font-size:11px}
.db-code{background:#f7f7f7;border:1px solid #eee;border-radius:6px;padding:10px 12px;
  font-size:12px;overflow-x:auto;margin:0}
</style>`
