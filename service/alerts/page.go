package alerts

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	store "mu/internal/alerts"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	gid := r.URL.Query().Get("group")
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		gid = r.PostFormValue("group")
		o, e := owner(acc.ID, gid, true)
		if e != nil {
			app.Forbidden(w, r, e.Error())
			return
		}
		if e = auth.CheckPostRate(acc.ID); e != nil {
			app.Error(w, r, 429, e.Error())
			return
		}
		if e = store.Update(o, r.PostFormValue("id"), r.PostFormValue("state")); e != nil {
			app.BadRequest(w, r, e.Error())
			return
		}
		http.Redirect(w, r, "/alerts"+scopeQuery(gid), http.StatusSeeOther)
		return
	}
	o, err := owner(acc.ID, gid, false)
	if err != nil {
		app.Forbidden(w, r, err.Error())
		return
	}
	items, err := store.List(o)
	if err != nil {
		app.Error(w, r, 503, "Alerts could not be loaded.")
		return
	}
	auth.SetCSRFCookie(w, r)
	history := r.URL.Query().Get("view") == "history"
	var b strings.Builder
	b.WriteString(`<div class="page-stack"><p class="text-muted">Conditions that need your attention. Acknowledging keeps an alert active; resolving or dismissing removes it from this list.</p><div class="form-actions"><a href="/inbox">Inbox</a><a href="/alerts` + scopeQuery(gid) + `">Active</a><a href="/alerts?view=history&amp;group=` + html.EscapeString(url.QueryEscape(gid)) + `">History</a></div>`)
	gs, _, err := group.List(acc.ID)
	if err == nil && len(gs) > 0 {
		b.WriteString(`<form method="get" action="/alerts" class="form"><label class="field-label">Show alerts for<select name="group"><option value="">Personal</option>`)
		for _, g := range gs {
			selected := ""
			if g.ID == gid {
				selected = " selected"
			}
			b.WriteString(`<option value="` + html.EscapeString(g.ID) + `"` + selected + `>` + html.EscapeString(g.Name) + `</option>`)
		}
		b.WriteString(`</select></label><div class="form-actions"><button type="submit">Show</button></div></form>`)
	}
	visible := []store.Item{}
	for _, i := range items {
		closed := i.State == "resolved" || i.State == "dismissed"
		if closed == history {
			visible = append(visible, i)
		}
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	offset = max(0, min(offset, len(visible)))
	end := min(offset+20, len(visible))
	b.WriteString(`<div class="collection-list">`)
	for _, i := range visible[offset:end] {
		b.WriteString(`<article class="collection-item"><div class="page-stack"><strong>` + html.EscapeString(i.Title) + `</strong><div class="metadata-row"><span>` + html.EscapeString(i.Source) + `</span><span>` + html.EscapeString(i.State) + `</span><span>` + html.EscapeString(app.TimeAgo(i.Updated)) + `</span></div><p>` + html.EscapeString(i.Detail) + `</p>`)
		if i.URL != "" {
			b.WriteString(`<a href="` + html.EscapeString(i.URL) + `">View details</a>`)
		}
		if !history && (gid == "" || group.Admin(gid, acc.ID)) {
			b.WriteString(`<form method="post" action="/alerts" class="form-actions">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="id" value="` + html.EscapeString(i.ID) + `"><input type="hidden" name="group" value="` + html.EscapeString(gid) + `">`)
			if i.State == "open" {
				b.WriteString(`<button name="state" value="acknowledged">Acknowledge</button>`)
			}
			b.WriteString(`<button name="state" value="resolved">Resolve</button><button name="state" value="dismissed">Dismiss</button></form>`)
		}
		b.WriteString(`</div></article>`)
	}
	b.WriteString(`</div>`)
	if len(visible) == 0 {
		b.WriteString(`<p class="text-muted">No alerts here.</p>`)
	}
	if end < len(visible) {
		view := ""
		if history {
			view = "history"
		}
		b.WriteString(fmt.Sprintf(`<a href="/alerts?group=%s&amp;view=%s&amp;offset=%d">More</a>`, html.EscapeString(url.QueryEscape(gid)), view, end))
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Alerts", HTML: b.String()})
}
func scopeQuery(id string) string {
	if id == "" {
		return ""
	}
	return "?group=" + url.QueryEscape(id)
}

// Preview is a bounded attention summary, not another activity stream. Group
// membership is checked on every render; no private payload enters a shared cache.
func Summary(actor string) string {
	if actor == "" {
		return ""
	}
	scopes := []struct{ owner, name, id string }{{actor, "Personal", ""}}
	gs, _, err := group.List(actor)
	if err == nil {
		for _, g := range gs {
			scopes = append(scopes, struct{ owner, name, id string }{group.Scope(g.ID), g.Name, g.ID})
		}
	}
	var b strings.Builder
	for _, s := range scopes {
		items, err := store.List(s.owner)
		if err != nil {
			continue
		}
		n := 0
		for _, i := range items {
			if i.State == "open" {
				n++
			}
		}
		if n > 0 {
			b.WriteString(`<li><a href="/alerts` + html.EscapeString(scopeQuery(s.id)) + `">` + html.EscapeString(s.name) + fmt.Sprintf(": %d alerts needing attention", n) + `</a></li>`)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return `<ul>` + b.String() + `</ul>`
}

// Preview wraps the same summary used by Home in an Inbox section.
func Preview(actor string) string {
	summary := Summary(actor)
	if summary == "" {
		return ""
	}
	return `<section class="record-card"><h2>Alerts</h2>` + summary + `</section>`
}
