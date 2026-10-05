// Library composes the account's existing records; it is not a message bookmark store.
package home

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
	"mu/service/files"
	"mu/service/shell"
)

const libraryDescription = "Your notes, documents, apps and files, created by you or Micro."

func LibraryHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	switch r.Method {
	case http.MethodGet:
		libraryView(w, r, acc)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form")
			return
		}
		if r.PostForm.Get("action") != "file" {
			app.BadRequest(w, r, "Unknown action")
			return
		}
		filePreview(w, r, acc)
	default:
		app.MethodNotAllowed(w, r)
	}
}

func libraryPreview(s string) string {
	r := []rune(s)
	if len(r) > 400 {
		return string(r[:400]) + "…"
	}
	return s
}

type libraryItem struct {
	title, kind, href, state string
	preview                  string
	updated                  time.Time
}

func libraryView(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	var b strings.Builder

	filter := r.URL.Query().Get("type")
	switch filter {
	case "", "note", "document", "app", "file":
	default:
		filter = ""
	}
	var items []libraryItem
	if filter == "" || filter == "note" {
		for _, n := range notes.All(acc.ID) {
			items = append(items, libraryItem{n.Title, "note", "/notes?id=" + url.QueryEscape(n.ID), "", libraryPreview(n.Text), n.UpdatedAt})
		}
	}
	if filter == "" || filter == "document" {
		for _, d := range docs.All(acc.ID, "", 500) {
			items = append(items, libraryItem{d.Title, "document", "/docs?id=" + url.QueryEscape(d.ID), "", "", d.Updated})
		}
	}
	if filter == "" || filter == "app" {
		for _, a := range apps.OwnedBy(acc.ID) {
			items = append(items, libraryItem{a.Name, "app", "/apps/" + url.PathEscape(a.Slug), "Saved", "", a.UpdatedAt})
		}
		for _, j := range apps.BuildsFor(acc.ID) {
			items = append(items, libraryItem{j.Title, "app", "/apps/builds/" + url.PathEscape(j.ID), j.State, "", j.Updated})
		}
	}
	if filter == "" || filter == "file" {
		for _, f := range files.List(acc.ID) {
			items = append(items, libraryItem{f.Name, "file", f.URL, "Saved", "", f.Created})
		}
	}
	controls := app.ViewNavigation("Library type", filter, []app.ViewLink{
		{Key: "", Label: "All", URL: "/home/library"},
		{Key: "note", Label: "Notes", URL: "/home/library?type=note"},
		{Key: "document", Label: "Docs", URL: "/home/library?type=document"},
		{Key: "app", Label: "Apps", URL: "/home/library?type=app"},
		{Key: "file", Label: "Files", URL: "/home/library?type=file"},
	}, true)
	b.WriteString(app.PageControls(libraryDescription, tabs(""), controls))
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].updated.Equal(items[j].updated) {
			return items[i].href < items[j].href
		}
		return items[i].updated.After(items[j].updated)
	})
	pager := app.Paginate(r, len(items), 25)
	if len(items) == 0 {
		b.WriteString(`<p class="text-muted">No notes, documents, apps or files yet. Create them in Services or ask Micro to help.</p>`)
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
	b.WriteString(`</div>` + pager.Nav("/home/library?type="+url.QueryEscape(filter)))
	if filter == "document" {
		b.WriteString(`<p><a href="/docs">All docs</a></p>`)
	}
	if shell.Configured() && (filter == "file" || filter == "") {
		b.WriteString(workspaceFiles(r, acc.ID))
	}
	app.Respond(w, r, app.Response{Title: "Library", HTML: `<div class="page-stack">` + b.String() + `</div>`})
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
		return `<p class="text-muted">Your workspace is asleep. Its files remain stored; opening Library does not start it.</p>`
	}
	var b strings.Builder
	b.WriteString(`<section class="record-card"><h2>Files</h2>`)
	count := 0
	for _, f := range ws.Files {
		if f.Dir || strings.HasPrefix(f.Name, ".") {
			continue
		}
		count++
		fmt.Fprintf(&b, `<form class="form-actions" method="post" action="/home/library"><input type="hidden" name="_csrf" value="%s"><input type="hidden" name="action" value="file"><input type="hidden" name="name" value="%s"><button type="submit">%s</button><span class="text-muted">%d bytes</span></form>`, html.EscapeString(auth.CSRFToken(r)), html.EscapeString(f.Name), html.EscapeString(f.Name), f.Size)
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
	body := app.PageControls(libraryDescription, tabs(""), "") + `<a href="/home/library?type=file">Back to files</a><h2>` + html.EscapeString(name) + `</h2>`
	if strings.ContainsRune(text, 0) || !utf8.ValidString(text) {
		body += `<p>This file has no text preview. Use your workspace to retrieve it.</p>`
	} else {
		body += `<pre>` + html.EscapeString(text) + `</pre>`
	}
	if truncated {
		body += `<p class="text-muted">Preview limited to 64 KB.</p>`
	}
	app.Respond(w, r, app.Response{Title: "Library", HTML: `<div class="page-stack">` + body + `</div>`})
}
