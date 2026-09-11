package home

import (
	"html"
	"strings"
	"time"

	"mu/internal/app"
	"mu/service/tasks"
)

// todoHTML uses current owned records; the brief never invents action items.
func todoHTML(accountID string) string {
	if accountID == "" {
		return ""
	}
	var rows []string
	total := 0
	for _, status := range []string{tasks.StatusBlocked, tasks.StatusFailed, tasks.StatusTodo} {
		for _, task := range tasks.List(accountID, status) {
			if status == tasks.StatusTodo && task.Assignee != tasks.Me {
				continue
			}
			total++
			if len(rows) == 5 {
				continue
			}
			label := "To do"
			switch status {
			case tasks.StatusBlocked:
				label = "Blocked · review what is needed"
			case tasks.StatusFailed:
				label = "Failed · review before retrying"
			default:
				if !task.Due.IsZero() {
					if task.Due.Before(time.Now()) {
						label = "Overdue"
					} else if sameDay(task.Due, time.Now()) {
						label = "Due today"
					}
				}
			}
			link := app.TextLink(html.EscapeString(task.Title), "/tasks?status="+status+"#task-"+task.ID)
			rows = append(rows, "<li>"+link+` <span class="text-muted">`+html.EscapeString(label)+"</span></li>")
		}
	}
	if total == 0 {
		return ""
	}
	return sectionRule("To do") + `<div class="brief-peek"><ul>` + strings.Join(rows, "") + `</ul>` + app.TextLink("View all tasks", "/tasks") + `</div>`
}
