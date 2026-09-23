package inbox

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
	"mu/service/apps"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/shell"
)

func viewNavigation(active string) string {
	var b strings.Builder
	b.WriteString(`<nav class="page-menu" aria-label="Inbox views">`)
	for _, v := range []struct{ key, name, href string }{{"conversations", "Messages", "/inbox"}, {"scheduled", "Scheduled", "/inbox?view=scheduled"}, {"saved", "Saved", "/inbox?view=saved"}, {"new", "New message", "/inbox/new"}} {
		current := ""
		if active == v.key {
			current = ` aria-current="page"`
		}
		fmt.Fprintf(&b, `<a href="%s"%s>%s</a>`, html.EscapeString(v.href), current, v.name)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// These views only compose records owned by the session account. They do not
// create another store or run the agent when a page is opened.
func collectionView(w http.ResponseWriter, r *http.Request, acc *auth.Account, view string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
			http.Error(w, "Invalid form", http.StatusForbidden)
			return
		}
		if view == "scheduled" && r.PostForm.Get("action") == "cancel" {
			if err := events.Cancel(acc.ID, r.PostForm.Get("id")); err != nil {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, "/inbox?view=scheduled", http.StatusSeeOther)
			return
		}
		if view == "saved" && r.PostForm.Get("action") == "file" {
			filePreview(w, r, acc)
			return
		}
		app.BadRequest(w, r, "Unknown action")
		return
	}
	if view == "scheduled" {
		scheduledView(w, r, acc)
		return
	}
	savedView(w, r, acc)
}

func scheduledView(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var b strings.Builder
	b.WriteString(viewNavigation("scheduled"))
	loc := time.UTC
	if z, err := time.LoadLocation(acc.Zone); err == nil {
		loc = z
	}
	b.WriteString(`<p class="text-muted">Reminders and work scheduled with Micro. Times shown in ` + html.EscapeString(loc.String()) + `.</p>`)
	for _, period := range []string{"morning", "evening"} {
		if events.Brief(acc.ID, period) != nil {
			continue
		}
		title := "Morning brief"
		if period == "evening" {
			title = "Evening Debrief"
		}
		fmt.Fprintf(&b, `<section class="record-card"><a class="record-title" href="/events?view=brief#%s-brief">%s</a><p class="text-muted">Not scheduled</p></section>`, period, title)
	}
	items := events.List(acc.ID)
	active := items[:0]
	for _, e := range items {
		if !e.Fired {
			active = append(active, e)
		}
	}
	pager := app.Paginate(r, len(active), shown)
	if len(active) == 0 {
		b.WriteString(`<p>Nothing scheduled. Ask Micro to remind you or do something later.</p>`)
	}
	b.WriteString(`<div class="collection-list">`)
	for _, e := range active[pager.From:pager.To] {
		kind, detail := "Reminder", "Sends a reminder to your subscribed devices."
		if e.Prompt != "" {
			kind, detail = "Agent work", e.Prompt
		}
		destination := "/events?id=" + url.QueryEscape(e.ID)
		if e.Kind == "brief" {
			kind, detail = "Brief", "Overnight developments and what matters today."
			if events.BriefPeriod(e) == "evening" {
				kind, detail = "Debrief", "Developments during the day and preparation for tomorrow."
			}
		}
		state := "Scheduled"
		if e.Paused {
			state = "Paused"
		}
		fmt.Fprintf(&b, `<section class="record-card"><a class="record-title" href="%s">%s</a><div class="metadata-row"><span>%s</span><span>%s</span><time datetime="%s">%s</time>`, html.EscapeString(destination), html.EscapeString(e.Title), kind, state, e.When.Format(time.RFC3339), html.EscapeString(e.When.In(loc).Format("Mon 2 Jan, 15:04 MST")))
		if e.Repeat != "" {
			b.WriteString(`<span>` + html.EscapeString(e.Repeat) + `</span>`)
		}
		b.WriteString(`</div><p class="collection-preview">` + html.EscapeString(trimTo(detail, 200)) + `</p>`)
		b.WriteString(`<div class="form-actions">`)
		if !e.Paused && e.Kind != "brief" {
			label := "Cancel"
			fmt.Fprintf(&b, `<form method="post" action="/inbox?view=scheduled"><input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="cancel"><input type="hidden" name="id" value="%s"><button type="submit">%s</button></form>`, html.EscapeString(auth.CSRFToken(r)), html.EscapeString(e.ID), label)
		}
		b.WriteString(`</div></section>`)
	}
	b.WriteString(`</div>` + pager.Nav("/inbox?view=scheduled"))
	b.WriteString(`<div class="section-actions"><a href="/events">Calendar and schedule settings</a></div>`)
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

type savedItem struct {
	title, kind, href, state string
	preview                  string
	updated                  time.Time
}

func savedView(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var b strings.Builder
	b.WriteString(viewNavigation("saved"))
	b.WriteString(`<p class="text-muted">Things you and Micro have saved. To create something, <a href="/?new=1">start a conversation</a>.</p>`)
	filter := r.URL.Query().Get("type")
	switch filter {
	case "", "note", "document", "app", "file":
	default:
		filter = ""
	}
	var items []savedItem
	if filter == "" || filter == "note" {
		for _, n := range notes.All(acc.ID) {
			items = append(items, savedItem{n.Title, "note", "/notes?id=" + url.QueryEscape(n.ID), "", trimTo(n.Text, 400), n.UpdatedAt})
		}
	}
	if filter == "" || filter == "document" {
		for _, d := range docs.All(acc.ID, "", 500) {
			items = append(items, savedItem{d.Title, "document", "/docs?id=" + url.QueryEscape(d.ID), "", "", d.Updated})
		}
	}
	if filter == "" || filter == "app" {
		for _, a := range apps.OwnedBy(acc.ID) {
			items = append(items, savedItem{a.Name, "app", "/apps/" + url.PathEscape(a.Slug), "Saved", "", a.UpdatedAt})
		}
		for _, j := range apps.BuildsFor(acc.ID) {
			items = append(items, savedItem{j.Title, "app", "/apps/builds/" + url.PathEscape(j.ID), j.State, "", j.Updated})
		}
	}
	// Filters remain small and textual; no dashboard or service catalogue.
	kinds := map[string]bool{}
	for _, item := range items {
		kinds[item.kind] = true
	}
	if shell.Configured() {
		kinds["file"] = true
	}
	if len(kinds) > 1 || filter != "" {
		b.WriteString(`<nav class="view-switch" aria-label="Saved type">`)
		for _, f := range []struct{ key, label string }{{"", "All"}, {"note", "Notes"}, {"document", "Docs"}, {"app", "Apps"}, {"file", "Files"}} {
			current := ""
			if filter == f.key {
				current = ` aria-current="page"`
			}
			fmt.Fprintf(&b, `<a href="/inbox?view=saved&amp;type=%s"%s>%s</a>`, f.key, current, f.label)
		}
		b.WriteString(`</nav>`)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].updated.Equal(items[j].updated) {
			return items[i].href < items[j].href
		}
		return items[i].updated.After(items[j].updated)
	})
	pager := app.Paginate(r, len(items), shown)
	if len(items) == 0 && filter != "file" {
		b.WriteString(`<p class="text-muted">Nothing saved here yet.</p>`)
	}
	b.WriteString(`<div class="collection-list">`)
	for _, item := range items[pager.From:pager.To] {
		kind := item.kind
		if kind == "document" {
			kind = "doc"
		}
		fmt.Fprintf(&b, `<div class="collection-item"><a class="collection-title" href="%s">%s</a><span class="collection-preview metadata-row"><span>%s</span><span>%s</span></span>`, html.EscapeString(item.href), html.EscapeString(item.title), html.EscapeString(kind), html.EscapeString(item.state))
		if item.preview != "" {
			b.WriteString(`<div class="collection-preview">` + string(app.RenderLinesNoImages([]byte(item.preview))) + `</div>`)
		}
		if !item.updated.IsZero() {
			fmt.Fprintf(&b, `<time class="collection-when" datetime="%s">%s</time>`, item.updated.Format(time.RFC3339), html.EscapeString(app.TimeAgo(item.updated)))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>` + pager.Nav("/inbox?view=saved&type="+url.QueryEscape(filter)))
	if filter == "document" {
		b.WriteString(`<p><a href="/docs">All docs</a></p>`)
	}
	if filter == "file" || (filter == "" && shell.Configured()) {
		b.WriteString(workspaceFiles(r, acc.ID))
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

func workspaceFiles(r *http.Request, owner string) string {
	if !shell.Configured() {
		return `<p class="text-muted">Workspace files are not available on this instance.</p>`
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	ws, err := shell.WorkspaceOf(ctx, owner)
	if err != nil {
		return `<p class="text-muted">Files are temporarily unavailable.</p>`
	}
	if !ws.Awake {
		return `<p class="text-muted">Your workspace is asleep. Its files remain stored; opening Saved does not start it.</p>`
	}
	var b strings.Builder
	b.WriteString(`<section class="record-card"><h2>Files</h2>`)
	count := 0
	for _, f := range ws.Files {
		if f.Dir || strings.HasPrefix(f.Name, ".") {
			continue
		}
		count++
		fmt.Fprintf(&b, `<form class="form-actions" method="post" action="/inbox?view=saved"><input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="file"><input type="hidden" name="name" value="%s"><button type="submit">%s</button><span class="text-muted">%d bytes</span></form>`, html.EscapeString(auth.CSRFToken(r)), html.EscapeString(f.Name), html.EscapeString(f.Name), f.Size)
	}
	if count == 0 {
		b.WriteString(`<p class="text-muted">No files at the top of your workspace.</p>`)
	}
	if ws.Total > len(ws.Files) {
		b.WriteString(`<p class="text-muted">Only the first workspace entries are shown.</p>`)
	}
	b.WriteString(`<a href="/shell">Open workspace</a></section>`)
	return b.String()
}

func filePreview(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	name := r.PostForm.Get("name")
	if !shell.Configured() || name == "" || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	text, truncated, err := shell.ReadFile(ctx, acc.ID, name, 64<<10)
	if err != nil {
		app.BadRequest(w, r, "Cannot open this file. The workspace may be asleep or the file may have been removed.")
		return
	}
	body := viewNavigation("saved") + `<a href="/inbox?view=saved&amp;type=file">Back to files</a><h2>` + html.EscapeString(name) + `</h2>`
	if strings.ContainsRune(text, 0) || !utf8.ValidString(text) {
		body += `<p>This file has no text preview. Use your workspace to retrieve it.</p>`
	} else {
		body += `<pre>` + html.EscapeString(text) + `</pre>`
	}
	if truncated {
		body += `<p class="text-muted">Preview limited to 64 KB.</p>`
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: body})
}
