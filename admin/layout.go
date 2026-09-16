package admin

import (
	"mu/internal/app"
	"net/http"
	"strings"
)

func respond(w http.ResponseWriter, r *http.Request, response app.Response) {
	var nav strings.Builder
	nav.WriteString(`<nav class="admin-navigation" aria-label="Admin">`)
	for _, item := range []struct{ path, label string }{
		{"/admin", "Overview"}, {"/admin/users", "Users"}, {"/admin/config", "Config"},
		{"/admin/traffic", "Usage"}, {"/admin/log", "Logs"}, {"/admin/status", "Status"},
		{"/admin/alerts", "Alerts"}, {"/admin/moderate", "Moderation"}, {"/admin/spam", "Spam"},
		{"/admin/server", "Server"}, {"/admin/backup", "Backup"}, {"/admin/oauth", "OAuth"},
	} {
		current := ""
		if r.URL.Path == item.path {
			current = ` aria-current="page"`
		}
		nav.WriteString(`<a href="` + item.path + `"` + current + `>` + item.label + `</a>`)
	}
	nav.WriteString(`</nav>`)
	response.BodyClass += " admin-page"
	response.HTML = nav.String() + response.HTML
	app.Respond(w, r, response)
}
