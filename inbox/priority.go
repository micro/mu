package inbox

import (
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
	all := arrivals(owner)
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
	path := r.URL.Path
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	return path
}

// priority renders the inbox list; a conversation opens only when selected.
func priority(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	auth.SetCSRFCookie(w, r)
	box := strings.Trim(strings.TrimPrefix(r.URL.Path, "/inbox"), "/")
	var b strings.Builder
	b.WriteString(searchBox("", strings.TrimSpace(r.PostFormValue("q")), auth.CSRFToken(r)))
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
			b.WriteString(row(r, owner, t))
		}
		b.WriteString(pager.Nav(r.URL.Path))
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

func waitingHTML(r *http.Request, owner string) string { return waiting(r, owner) }
