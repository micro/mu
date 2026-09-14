package inbox

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// priority presents one communication at a time. Tasks belong in Work; mail envelopes in Mail.
func priority(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	auth.SetCSRFCookie(w, r)
	box := strings.Trim(strings.TrimPrefix(r.URL.Path, "/inbox"), "/")
	history := box != "" || r.URL.Query().Get("view") == "history" || r.Method == http.MethodPost
	var b strings.Builder
	b.WriteString(`<div class="toolbar"><a href="/">Talk to Micro</a><a href="/inbox?view=history">History</a></div>`)
	if history {
		b.WriteString(searchBox("", strings.TrimSpace(r.PostFormValue("q")), auth.CSRFToken(r)))
		if q := strings.TrimSpace(r.PostFormValue("q")); q != "" {
			found(&b, r, owner, box, q)
			app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
			return
		}
		all := arrivals(owner)
		if box != "" {
			filtered := all[:0:0]
			for _, t := range all {
				if strings.EqualFold(boxOfThread(owner, t), box) {
					filtered = append(filtered, t)
				}
			}
			all = filtered
		}
		pager := app.Paginate(r, len(all), shown)
		for _, t := range all[pager.From:pager.To] {
			b.WriteString(row(r, owner, t))
		}
		b.WriteString(pager.Nav("/inbox?view=history"))
	} else {
		var waiting []thread.Thread
		for _, t := range arrivals(owner) {
			if t.Updated.After(t.Handled) {
				waiting = append(waiting, t)
			}
		}
		b.WriteString(waitingHTML(r, owner))
		if len(waiting) == 0 {
			b.WriteString(`<p class="message">Nothing needs your attention.</p>`)
		} else {
			t := waiting[0]
			b.WriteString(`<article class="message"><p class="text-muted">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</p><h2>` + html.EscapeString(t.Subject) + `</h2>`)
			msgs := thread.Messages(owner, t.ID, 1)
			if len(msgs) > 0 {
				text, _ := unquoted(msgs[0].Text)
				b.WriteString(`<div class="markdown-content">` + app.RenderString(text) + `</div>`)
			}
			b.WriteString(`<div class="form-actions"><a class="btn btn-secondary" href="/inbox?id=` + url.QueryEscape(t.ID) + `">Open conversation</a><button type="button" onclick="muAssignOpen()">Ask Micro</button><form class="form-action" method="POST" action="/inbox">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `"><input type="hidden" name="reviewed" value="` + t.Updated.Format(time.RFC3339Nano) + `"><input type="hidden" name="action" value="handled"><button>Done</button></form></div>`)
			b.WriteString(assignDialog(r, owner, &t, ""))
			b.WriteString(`</article>`)
			if len(waiting) > 1 {
				b.WriteString(`<p class="text-muted">More communications are waiting. Mark this done to see the next.</p>`)
			}
		}
	}
	app.Respond(w, r, app.Response{Title: "Inbox", HTML: b.String()})
}

func waitingHTML(r *http.Request, owner string) string { return waiting(r, owner) }
