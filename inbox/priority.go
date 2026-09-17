package inbox

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
)

// inboxThreads is the same ordered selection for the list and its reader.
func inboxThreads(owner, path string) []thread.Thread {
	box := strings.Trim(strings.TrimPrefix(path, "/inbox"), "/")
	all := thread.List(owner, held)
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
	for _, key := range []string{"view", "page"} {
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
	w.Header().Set("Cache-Control", "no-store")
	auth.SetCSRFCookie(w, r)
	box := strings.Trim(strings.TrimPrefix(r.URL.Path, "/inbox"), "/")
	var b strings.Builder
	b.WriteString(viewNavigation("conversations"))
	b.WriteString(`<div class="section-actions"><a href="/?new=1">New conversation</a></div>`)
	b.WriteString(searchBox(box, strings.TrimSpace(r.PostFormValue("q")), auth.CSRFToken(r)))
	b.WriteString(waitingHTML(r, owner))
	if q := strings.TrimSpace(r.PostFormValue("q")); q != "" {
		found(&b, r, owner, box, q)
	} else {
		all := inboxThreads(owner, r.URL.Path)
		pager := app.Paginate(r, len(all), shown)
		if len(all) == 0 {
			b.WriteString(`<p class="text-muted">Your inbox is empty.</p>`)
		}
		for _, t := range all[pager.From:pager.To] {
			b.WriteString(conversationRow(t, ""))
		}
		b.WriteString(pager.Nav(r.URL.Path))
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

func waitingHTML(r *http.Request, owner string) string { return waiting(r, owner) }

// conversationRow keeps saved conversations compact and easy to return to.
func conversationRow(t thread.Thread, preview string) string {
	title := strings.TrimSpace(t.Subject)
	if title == "" {
		title = "Untitled conversation"
	}
	destination := "/inbox?id=" + url.QueryEscape(t.ID)
	if t.Client == thread.WebClient {
		destination = "/?session=" + url.QueryEscape(t.ID)
	}
	unread := ""
	if thread.Unread(t) {
		unread = `<span class="unread-dot" aria-label="Unread"></span>`
	}
	result := `<a class="conversation-row" href="` + html.EscapeString(destination) + `"><span class="conversation-title">` + unread + html.EscapeString(title) + `</span><time datetime="` + t.Updated.Format("2006-01-02T15:04:05Z07:00") + `">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</time>`
	if preview != "" {
		result += `<span class="conversation-preview">` + html.EscapeString(preview) + `</span>`
	}
	return result + `</a>`
}
