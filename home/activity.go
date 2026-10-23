package home

import (
	"mu/agent"
	"mu/agent/work"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func activityPage(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	w.Header().Set("Cache-Control", "private, no-store")
	rows := recentActivity(acc.ID)
	pager := app.Paginate(r, len(rows), 25)
	var b strings.Builder
	b.WriteString(app.PageControls("", tabs("overview"), `<div class="form-actions"><a class="btn" href="/agent?view=new-chat">New chat</a></div>`))
	if len(rows) == 0 {
		b.WriteString(`<p class="text-muted">Your conversations, incoming messages and work will appear here.</p>`)
	}
	for _, row := range rows[pager.From:pager.To] {
		b.WriteString(inbox.ChatRow(row))
	}
	b.WriteString(pager.Nav("/home?view=activity"))
	app.Respond(w, r, app.Response{Title: "Recent activity", HTML: `<div class="page-stack">` + b.String() + `</div>`})
}

// Scheduled failures remain visible when no notification was delivered.
func recentActivity(owner string) []inbox.Chat {
	rows := agent.RecentActivity(owner, true)
	for _, t := range work.ScheduledAttention(owner) {
		rows = append(rows, inbox.Chat{ID: "work:" + t.ID, Title: t.Title, Context: "Scheduled", URL: "/work?id=" + url.QueryEscape(t.ID), Status: t.Status, Updated: t.Updated, Preview: inbox.PreviewText(t.Occurrence.Failure)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Updated.Equal(rows[j].Updated) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Updated.After(rows[j].Updated)
	})
	return rows
}
