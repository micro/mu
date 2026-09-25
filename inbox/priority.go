package inbox

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// inboxThreads is the same ordered selection for the list and its reader.
func inboxThreads(owner, path string) []thread.Thread {
	box := strings.Trim(strings.TrimPrefix(path, "/inbox"), "/")
	all := thread.List(owner, 0)
	if box == "" {
		return all
	}
	out := make([]thread.Thread, 0, len(all))
	for _, t := range all {
		if strings.EqualFold(boxOfThread(owner, t), box) {
			out = append(out, t)
		}
	}
	return out
}

func inboxURL(r *http.Request, id string) string {
	q := url.Values{}
	for _, key := range []string{"view", "page", "filter"} {
		if v := r.URL.Query().Get(key); v != "" {
			q.Set(key, v)
		}
	}
	if id != "" {
		q.Set("id", id)
	}
	u := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
	return u.String()
}

// priority renders the inbox list; a conversation opens only when selected.
func priority(w http.ResponseWriter, r *http.Request, owner string) {
	reviewed := time.Now().UTC()
	w.Header().Set("Cache-Control", "no-store")
	auth.SetCSRFCookie(w, r)
	box := strings.Trim(strings.TrimPrefix(r.URL.Path, "/inbox"), "/")
	var b strings.Builder
	b.WriteString(viewNavigation("conversations"))
	b.WriteString(searchBox(box, strings.TrimSpace(r.PostFormValue("q")), auth.CSRFToken(r), unreadOnly(r)))
	b.WriteString(`<nav class="form-actions" aria-label="Conversation filter">` + app.PillLink("All", r.URL.Path, !unreadOnly(r)) + app.PillLink("Unread", r.URL.Path+"?filter=unread", unreadOnly(r)) + `</nav>`)
	if r.Method == http.MethodGet {
		b.WriteString(`<div data-inbox-list>`)
	} else {
		b.WriteString(`<div>`)
	}
	b.WriteString(waitingHTML(r, owner))
	if q := strings.TrimSpace(r.PostFormValue("q")); q != "" {
		found(&b, r, owner, box, q)
	} else {
		all := filterUnread(r, inboxThreads(owner, r.URL.Path))
		pager := app.Paginate(r, len(all), shown)
		if len(all) == 0 {
			if unreadOnly(r) {
				b.WriteString(`<p class="text-muted">You have no unread conversations.</p>`)
			} else {
				b.WriteString(`<p class="text-muted">Your inbox is empty.</p>`)
			}
		}
		unread := 0
		for _, t := range all {
			if thread.Unread(t) {
				unread++
			}
		}
		if unread > 0 {
			b.WriteString(`<form method="post" action="` + html.EscapeString(r.URL.Path) + `" class="bulk-read">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="action" value="mark_read"><input type="hidden" name="reviewed" value="` + reviewed.Format(time.RFC3339Nano) + `">`)
			if unreadOnly(r) {
				b.WriteString(`<input type="hidden" name="filter" value="unread">`)
			}
			b.WriteString(fmt.Sprintf(`<div class="form-actions"><button name="scope" value="selected">Mark selected as read</button><button name="scope" value="all">Mark all %d as read</button></div>`, unread))
		}
		for _, t := range all[pager.From:pager.To] {
			if unread > 0 {
				b.WriteString(`<div class="selectable-row">`)
				if thread.Unread(t) {
					b.WriteString(`<input type="checkbox" name="id" value="` + html.EscapeString(t.ID) + `" aria-label="Select ` + html.EscapeString(t.Subject) + `">`)
				} else {
					b.WriteString(`<span></span>`)
				}
			}
			b.WriteString(conversationRow(r, owner, t, ""))
			if unread > 0 {
				b.WriteString(`</div>`)
			}
		}
		if unread > 0 {
			b.WriteString(`</form>`)
		}
		b.WriteString(pager.Nav(r.URL.Path))
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

func waitingHTML(r *http.Request, owner string) string { return waiting(r, owner) }

// conversationRow keeps saved conversations compact and easy to return to.
func conversationRow(r *http.Request, owner string, t thread.Thread, preview string) string {
	title := strings.TrimSpace(t.Subject)
	if title == "" {
		title = "Untitled conversation"
	}
	destination := inboxURL(r, t.ID)
	if t.Client == thread.WebClient {
		destination = "/?session=" + url.QueryEscape(t.ID)
	}
	unread := ""
	if thread.Unread(t) {
		unread = `<span class="unread-dot" aria-label="Unread"></span>`
	}
	result := `<a class="conversation-row" href="` + html.EscapeString(destination) + `"><span class="conversation-title">` + unread + html.EscapeString(title) + `</span><time datetime="` + t.Updated.Format("2006-01-02T15:04:05Z07:00") + `">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</time>`
	who, _ := party(owner, t)
	result += `<span class="conversation-context"><span class="metadata-kind">` + html.EscapeString(app.ClientName(t.Client)) + `</span><span>` + html.EscapeString(who) + `</span></span>`
	if preview != "" {
		result += `<span class="conversation-preview">` + html.EscapeString(preview) + `</span>`
	}
	return result + `</a>`
}

func unreadOnly(r *http.Request) bool { return r.URL.Query().Get("filter") == "unread" }

func filterUnread(r *http.Request, all []thread.Thread) []thread.Thread {
	if !unreadOnly(r) {
		return all
	}
	out := make([]thread.Thread, 0, len(all))
	for _, t := range all {
		if thread.Unread(t) {
			out = append(out, t)
		}
	}
	return out
}
