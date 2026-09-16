package app

import "strings"

func adminNavigation(path string) string {
	var nav strings.Builder
	nav.WriteString(`<nav class="admin-navigation" aria-label="Admin">`)
	for _, item := range []struct{ path, label string }{
		{"/admin", "Overview"}, {"/admin/users", "Users"}, {"/admin/config", "Config"},
		{"/admin/work", "Work"}, {"/admin/traffic", "Usage"}, {"/admin/log", "Logs"}, {"/admin/status", "Status"},
		{"/admin/alerts", "Alerts"}, {"/admin/moderate", "Moderation"}, {"/admin/spam", "Spam"},
		{"/admin/server", "Server"}, {"/admin/backup", "Backup"}, {"/admin/oauth", "OAuth"},
	} {
		current := ""
		if path == item.path {
			current = ` aria-current="page"`
		}
		nav.WriteString(`<a href="` + item.path + `"` + current + `>` + item.label + `</a>`)
	}
	nav.WriteString(`</nav>`)
	return nav.String()
}
