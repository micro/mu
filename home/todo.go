package home

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"mu/account"
	"mu/agent"
	"mu/internal/app"
	"mu/service/tasks"
	"mu/work"
)

// Todo is a view of outstanding actions, not a second task store. Reading a
// message does not complete an action; replying or changing its work state does.
func todoHTML(owner string) string {
	var rows []string
	add := func(title, href, detail string) {
		rows = append(rows, `<a class="todo-row" href="`+html.EscapeString(href)+`"><span class="todo-mark" aria-hidden="true">○</span><span class="collection-title">`+html.EscapeString(title)+`</span><span class="collection-preview">`+html.EscapeString(detail)+`</span></a>`)
	}
	if th := agent.PendingCheckin(owner, account.LocalNow(owner)); th != nil {
		add("Respond to today’s check-in", "/checkin?id="+url.QueryEscape(th.ID), "A sentence or two is enough")
	}
	for _, t := range append(work.ScheduledAttention(owner), tasks.List(owner, "")...) {
		if !t.Open() {
			continue
		}
		label := ""
		switch t.Status {
		case tasks.StatusBlocked:
			label = "Needs your input"
		case tasks.StatusFailed:
			label = "Review failed work"
		case tasks.StatusTodo, tasks.StatusDoing:
			if t.Assignee == tasks.Me {
				label = "Your task"
			}
		}
		if label != "" {
			add(t.Title, "/work?id="+url.QueryEscape(t.ID), label)
		}
	}
	if len(rows) == 0 {
		return ""
	}
	shown := min(len(rows), 5)
	body := `<div class="compact-list">` + strings.Join(rows[:shown], "") + `</div>`
	if len(rows) > shown {
		body += fmt.Sprintf(`<p><a href="/work">%d more in Work</a></p>`, len(rows)-shown)
	}

	return app.PreviewSection("home-todo", "Todo", "/work", body)
}
