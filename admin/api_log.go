package admin

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// APILogHandler shows the external API call log page.
func APILogHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	entries := app.APILog()

	var content strings.Builder

	content.WriteString(`<div class="card">`)
	content.WriteString(fmt.Sprintf(`<h3>External API Calls <span class="count">%d</span></h3>`, len(entries)))

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No API calls recorded yet.</p>`)
	} else {
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Service</th><th>Method</th><th class="hide-mobile">URL</th><th>Status</th><th>Duration</th><th>Error</th></tr>`)

		for _, e := range entries {
			statusClass := "dir-int"
			statusLabel := fmt.Sprintf("%d", e.Status)
			if e.Status == 0 {
				statusLabel = "err"
				statusClass = "dir-out"
			} else if e.Status >= 200 && e.Status < 300 {
				statusClass = "dir-in"
			} else if e.Status >= 400 {
				statusClass = "dir-out"
			}

			errStr := ""
			if e.Error != "" {
				errStr = truncate(e.Error, 60)
			}

			content.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td class="addr hide-mobile" title="%s">%s</td>
				<td class="%s">%s</td>
				<td>%dms</td>
				<td class="subject" title="%s">%s</td>
			</tr>`,
				e.Time.Format("Jan 2 15:04:05"),
				e.Service,
				e.Method,
				e.URL, truncate(e.URL, 50),
				statusClass, statusLabel,
				e.Duration.Milliseconds(),
				e.Error, errStr,
			))

			if e.RequestBody != "" || e.ResponseBody != "" {
				content.WriteString(`<tr><td colspan="7">`)
				if e.RequestBody != "" {
					content.WriteString(fmt.Sprintf(`<details><summary>Request</summary><pre class="raw-sm">%s</pre></details>`, html.EscapeString(e.RequestBody)))
				}
				if e.ResponseBody != "" {
					content.WriteString(fmt.Sprintf(`<details><summary>Response</summary><pre class="raw-sm">%s</pre></details>`, html.EscapeString(e.ResponseBody)))
				}
				content.WriteString(`</td></tr>`)
			}
		}

		content.WriteString(`</table>`)
	}

	content.WriteString(`</div>`)
	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	app.Respond(w, r, app.Response{Title: "API Log", Description: "External API Log", HTML: content.String()})
}
