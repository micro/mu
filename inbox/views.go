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
	"mu/service/files"
	"mu/service/shell"
)

const inboxDescription = "Read and reply to messages, and find things you and Micro have saved."

func viewNavigation(active string) string {
	return app.ViewNavigation("Inbox views", active, []app.ViewLink{
		{Key: "conversations", Label: "Messages", URL: "/inbox"},
		{Key: "saved", Label: "Saved", URL: "/inbox?view=saved"},
	}, false)
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
	http.Redirect(w, r, "/agents?view=scheduled", http.StatusSeeOther)
}

type savedItem struct {
	title, kind, href, state string
	preview                  string
	updated                  time.Time
}

func savedView(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var b strings.Builder

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
	if filter == "" || filter == "file" {
		for _, f := range files.List(acc.ID) {
			items = append(items, savedItem{f.Name, "file", f.URL, "Saved", "", f.Created})
		}
	}
	controls := `<div class="form-actions"><a class="btn" href="/inbox/new">New message</a></div>` + viewNavigation("saved") + app.ViewNavigation("Saved type", filter, []app.ViewLink{
		{Key: "", Label: "All", URL: "/inbox?view=saved"},
		{Key: "note", Label: "Notes", URL: "/inbox?view=saved&type=note"},
		{Key: "document", Label: "Docs", URL: "/inbox?view=saved&type=document"},
		{Key: "app", Label: "Apps", URL: "/inbox?view=saved&type=app"},
		{Key: "file", Label: "Files", URL: "/inbox?view=saved&type=file"},
	}, true)
	b.WriteString(app.PageControls(inboxDescription, "", controls))
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].updated.Equal(items[j].updated) {
			return items[i].href < items[j].href
		}
		return items[i].updated.After(items[j].updated)
	})
	pager := app.Paginate(r, len(items), shown)
	if len(items) == 0 {
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
	if shell.Configured() && (filter == "file" || filter == "") {
		b.WriteString(workspaceFiles(r, acc.ID))
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: `<div class="page-stack">` + b.String() + `</div>`})
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
	body := app.PageControls(inboxDescription, viewNavigation("saved"), "") + `<a href="/inbox?view=saved&amp;type=file">Back to files</a><h2>` + html.EscapeString(name) + `</h2>`
	if strings.ContainsRune(text, 0) || !utf8.ValidString(text) {
		body += `<p>This file has no text preview. Use your workspace to retrieve it.</p>`
	} else {
		body += `<pre>` + html.EscapeString(text) + `</pre>`
	}
	if truncated {
		body += `<p class="text-muted">Preview limited to 64 KB.</p>`
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: `<div class="page-stack">` + body + `</div>`})
}
