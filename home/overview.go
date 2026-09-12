package home

import (
	"mu/internal/app"
	"mu/service/events"
	"mu/service/tasks"
	"strings"
)

// overviewHTML reads owned records and the cached brief; rendering never calls a model.
func overviewHTML(owner string, external ...events.External) string {
	parts := briefParts(owner, external...)
	if owner != "" {
		attention := len(tasks.List(owner, tasks.StatusBlocked)) + len(tasks.List(owner, tasks.StatusFailed))
		if attention > 0 {
			parts = append(parts, app.TextLink(count(attention, "thing needs", "things need")+" your attention", "/tasks")+".")
		}

		n := 0
		for _, task := range tasks.List(owner, tasks.StatusTodo) {
			if task.Assignee == tasks.Me {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, "You have "+app.TextLink(count(n, "thing", "things")+" to do", "/tasks?status=todo")+".")
		}
	}
	if len(parts) == 0 {
		return `<section aria-labelledby="overview-title"><h2 id="overview-title">Today</h2><p>What would you like help with?</p></section>`
	}
	return `<section aria-labelledby="overview-title"><h2 id="overview-title">Today</h2><p>` + strings.Join(parts, "</p><p>") + `</p></section>`
}
