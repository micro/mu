package admin

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
)

// SysLogHandler shows the in-memory system log page.
func SysLogHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	entries := app.GetSysLog()

	var content strings.Builder
	content.WriteString(`<div class="card">`)
	content.WriteString(fmt.Sprintf(`<h3>System Log <span class="count">%d</span></h3>`, len(entries)))

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No log entries yet.</p>`)
	} else {
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Package</th><th>Message</th></tr>`)
		for _, e := range entries {
			content.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td class="subject">%s</td>
			</tr>`,
				e.Time.Format("Jan 2 15:04:05"),
				html.EscapeString(e.Package),
				html.EscapeString(e.Message),
			))
		}
		content.WriteString(`</table>`)
	}

	content.WriteString(`</div>`)
	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	pageHTML := app.RenderHTMLForRequest("System Log", "System Log", content.String(), r)
	w.Write([]byte(pageHTML))
}
