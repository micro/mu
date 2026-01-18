package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
	"mu/mail"
)

// EmailLogHandler shows the email log page
func EmailLogHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	logs := mail.GetEmailLogs(100)
	stats := mail.GetEmailLogStats()

	var content strings.Builder

	// Stats summary
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Email Statistics</h3>`)
	content.WriteString(`<table style="width: 100%; font-size: 14px;">`)
	content.WriteString(fmt.Sprintf(`<tr><td>Last 24 hours</td><td style="text-align: right;">%d total (%d outbound)</td></tr>`, stats["last_24h"], stats["last_24h_out"]))
	content.WriteString(fmt.Sprintf(`<tr><td>Total logged</td><td style="text-align: right;">%d</td></tr>`, stats["total"]))
	content.WriteString(fmt.Sprintf(`<tr><td>Inbound</td><td style="text-align: right;">%d</td></tr>`, stats["inbound"]))
	content.WriteString(fmt.Sprintf(`<tr><td>Outbound</td><td style="text-align: right;">%d</td></tr>`, stats["outbound"]))
	content.WriteString(fmt.Sprintf(`<tr><td>Failed</td><td style="text-align: right;">%d</td></tr>`, stats["failed"]))
	content.WriteString(`</table>`)
	content.WriteString(`</div>`)

	// Log entries
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Recent Emails</h3>`)
	
	if len(logs) == 0 {
		content.WriteString(`<p style="color: #666;">No emails logged yet.</p>`)
	} else {
		content.WriteString(`<style>
			.email-log { width: 100%; border-collapse: collapse; font-size: 13px; }
			.email-log th { text-align: left; padding: 8px; border-bottom: 2px solid #ddd; }
			.email-log td { padding: 8px; border-bottom: 1px solid #eee; vertical-align: top; }
			.email-log .dir-in { color: #22c55e; }
			.email-log .dir-out { color: #3b82f6; }
			.email-log .status-failed { color: #ef4444; }
			.email-log .status-sent, .email-log .status-received { color: #22c55e; }
			.email-log .subject { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
			.email-log .addr { max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: monospace; font-size: 12px; }
			@media (max-width: 768px) {
				.email-log .subject, .email-log .addr { max-width: 100px; }
			}
		</style>`)
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Dir</th><th>From</th><th>To</th><th>Subject</th><th>Status</th></tr>`)

		for _, log := range logs {
			dirClass := "dir-out"
			dirLabel := "→"
			if log.Direction == "inbound" {
				dirClass = "dir-in"
				dirLabel = "←"
			}

			statusClass := "status-" + log.Status
			statusLabel := log.Status
			if log.Error != "" {
				statusLabel = fmt.Sprintf(`<span title="%s">%s</span>`, log.Error, log.Status)
			}

			content.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td class="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="subject" title="%s">%s</td>
				<td class="%s">%s</td>
			</tr>`,
				log.Timestamp.Format("Jan 2 15:04:05"),
				dirClass, dirLabel,
				log.From, truncateAddr(log.From),
				log.To, truncateAddr(log.To),
				log.Subject, truncateSubject(log.Subject),
				statusClass, statusLabel,
			))
		}

		content.WriteString(`</table>`)
	}
	content.WriteString(`</div>`)

	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	html := app.RenderHTMLForRequest("Email Log", "Email activity log", content.String(), r)
	w.Write([]byte(html))
}

func truncateAddr(s string) string {
	if len(s) > 25 {
		return s[:22] + "..."
	}
	return s
}

func truncateSubject(s string) string {
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}
