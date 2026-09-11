package home

import (
	"html"
	"strconv"
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
			label := "Todo"
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
			link := "/tasks?status=" + status + "#task-" + task.ID
			rows = append(rows, `<a class="peek-row" href="`+html.EscapeString(link)+`"><span class="peek-head"><span class="peek-title">`+html.EscapeString(task.Title)+`</span></span><span class="peek-line">`+html.EscapeString(label)+`</span></a>`)
		}
	}
	if total == 0 {
		return ""
	}
	more := ""
	if n := total - len(rows); n > 0 {
		more = app.SectionLink(strconv.Itoa(n)+" more", "/tasks")
	}
	return sectionRule("Todo") + `<div class="todo-peek">` + strings.Join(rows, "") + `</div>` + more
}
