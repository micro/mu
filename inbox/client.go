package inbox

import (
	"mu/internal/app"
	"mu/internal/result"
	"mu/internal/thread"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type clientRow struct {
	ID      string    `json:"id"`
	Subject string    `json:"subject"`
	Sender  string    `json:"sender"`
	Kind    string    `json:"kind"`
	Preview string    `json:"preview"`
	Updated time.Time `json:"updated"`
	Unread  bool      `json:"unread"`
}

func summary(owner string, t thread.Thread) clientRow {
	who, _ := party(owner, t)
	preview := ""
	if ms := thread.Messages(owner, t.ID, 1); len(ms) > 0 {
		preview = plainPreview(ms[len(ms)-1].Text)
	}
	return clientRow{t.ID, t.Subject, who, t.Client, preview, t.Updated, thread.Unread(t)}
}

func clientData(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "private, no-store")
	all := inboxThreads(owner, r.URL.Path)
	if id := r.URL.Query().Get("id"); id != "" {
		t := thread.Get(owner, id)
		if t == nil {
			app.NotFound(w, r, "Conversation not found")
			return
		}
		row := summary(owner, *t)
		offset, _ := strconv.Atoi(r.URL.Query().Get("before"))
		ms, hasOlder := thread.MessageWindow(owner, id, offset, MessagesShown)
		type message struct {
			ID      string        `json:"id"`
			Role    string        `json:"role"`
			Text    string        `json:"text"`
			From    string        `json:"from"`
			At      time.Time     `json:"at"`
			Results []result.Item `json:"results,omitempty"`
			HTML    string        `json:"email_html,omitempty"`
		}
		out := make([]message, 0, len(ms))
		for _, m := range ms {
			rendered := ""
			if t.Client == mailClient {
				rendered = mailBody(owner, m)
			}
			out = append(out, message{m.ID, m.Role, m.Text, m.From, m.At, m.Results, rendered})
		}
		prev, next := "", ""
		for i, t := range all {
			if t.ID == id {
				if i > 0 {
					prev = all[i-1].ID
				}
				if i+1 < len(all) {
					next = all[i+1].ID
				}
				break
			}
		}
		thread.MarkSeen(owner, id)
		app.RespondJSON(w, map[string]any{"thread": row, "messages": out, "reply_to": replyTo(owner, t, ms), "previous": prev, "next": next, "has_older": hasOlder, "before": offset + len(ms), "open": room(t)})
		return
	}
	q := strings.TrimSpace(r.PostFormValue("q"))
	if q != "" {
		found := map[string]bool{}
		for _, hit := range thread.Search(owner, q, "", held) {
			found[hit.Thread] = true
		}
		filtered := all[:0:0]
		for _, t := range all {
			if found[t.ID] {
				filtered = append(filtered, t)
			}
		}
		all = filtered
	}
	p := app.Paginate(r, len(all), shown)
	rows := make([]clientRow, 0, p.To-p.From)
	for _, t := range all[p.From:p.To] {
		rows = append(rows, summary(owner, t))
	}
	app.RespondJSON(w, map[string]any{"items": rows, "page": p.Page, "total": len(all), "page_size": shown, "query": q})
}

func plainPreview(text string) string {
	text = strings.Join(strings.Fields(plain(text)), " ")
	if utf8.RuneCountInString(text) > 180 {
		text = string([]rune(text)[:180]) + "…"
	}
	return text
}
