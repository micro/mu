package inbox

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"sort"
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
	filters := app.ViewNavigation("Inbox views", messageFilter(r), []app.ViewLink{
		{Key: "all", Label: "All", URL: r.URL.Path},
		{Key: "unread", Label: "Unread", URL: r.URL.Path + "?filter=unread"},
		{Key: "saved", Label: "Saved", URL: r.URL.Path + "?filter=saved"},
		{Key: "sent", Label: "Sent", URL: r.URL.Path + "?filter=sent"},
		{Key: "junk", Label: "Junk", URL: "/inbox?view=junk"},
		{Key: "settings", Label: "Settings", URL: "/inbox?view=mail-settings"},
	}, false)
	controls := `<div class="form-actions"><a class="btn" href="/inbox/new">New message</a><a class="btn" href="/agent?view=new-chat">New chat</a></div>` + searchBox(box, strings.TrimSpace(r.PostFormValue("q")), auth.CSRFToken(r), messageFilter(r))
	b.WriteString(app.PageControls(inboxDescription, filters, controls))
	if r.Method == http.MethodGet {
		b.WriteString(`<div data-inbox-list>`)
	} else {
		b.WriteString(`<div>`)
	}
	b.WriteString(waitingHTML(r, owner))
	if box == "" && messageFilter(r) == "all" {
		b.WriteString(invitations(owner, auth.CSRFToken(r)))
	}
	if q := strings.TrimSpace(r.PostFormValue("q")); q != "" {
		found(&b, r, owner, box, q)
	} else {
		all := filterThreads(r, inboxThreads(owner, r.URL.Path))
		type entry struct {
			t    *thread.Thread
			chat *Chat
			at   time.Time
		}
		var entries []entry
		for i := range all {
			entries = append(entries, entry{t: &all[i], at: all[i].Updated})
		}
		if box == "" && (messageFilter(r) == "all" || unreadOnly(r)) {
			for _, c := range Chats(owner) {
				if !c.Shared || (unreadOnly(r) && !c.Unread) {
					continue
				}
				c := c
				entries = append(entries, entry{chat: &c, at: c.Updated})
			}
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].at.After(entries[j].at) })
		pager := app.Paginate(r, len(entries), shown)
		if len(entries) == 0 {
			if messageFilter(r) == "saved" {
				b.WriteString(`<p class="text-muted">No saved conversations. Open a conversation and choose Save to keep it here.</p>`)
			} else if messageFilter(r) == "sent" {
				b.WriteString(`<p class="text-muted">Conversations you have sent messages to will appear here.</p>`)
			} else if unreadOnly(r) {
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
			b.WriteString(`<input type="hidden" name="filter" value="` + messageFilter(r) + `">`)
			b.WriteString(fmt.Sprintf(`<div class="form-actions"><button name="scope" value="selected">Mark selected as read</button><button name="scope" value="all">Mark all %d personal conversations as read</button></div>`, unread))
		}
		for _, item := range entries[pager.From:pager.To] {
			if item.chat != nil {
				b.WriteString(ChatRow(*item.chat))
				continue
			}
			t := *item.t
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
		b.WriteString(pager.Nav(inboxURL(r, "")))
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: `<div class="section-body">` + b.String() + `</div>`})
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

func unreadOnly(r *http.Request) bool { return messageFilter(r) == "unread" }

func messageFilter(r *http.Request) string {
	f := r.URL.Query().Get("filter")
	if f == "" {
		f = r.PostFormValue("filter")
	}
	switch f {
	case "unread", "saved", "sent":
		return f
	}
	return "all"
}

func matchesFilter(r *http.Request, t thread.Thread) bool {
	switch messageFilter(r) {
	case "unread":
		return thread.Unread(t)
	case "saved":
		return t.Saved
	case "sent":
		return thread.Sent(t.Account, t.ID)
	}
	return true
}

func filterThreads(r *http.Request, all []thread.Thread) []thread.Thread {
	out := make([]thread.Thread, 0, len(all))
	for _, t := range all {
		if matchesFilter(r, t) {
			out = append(out, t)
		}
	}
	return out
}
