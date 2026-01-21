package admin

import (
	"fmt"
	"net/http"
	"sort"
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

	stats := mail.GetEmailStats()
	messages := mail.GetRecentMessages(50)

	// Sort domains by count
	type domainCount struct {
		Domain string
		Count  int
	}
	var sortedDomains []domainCount
	for d, c := range stats.Domains {
		sortedDomains = append(sortedDomains, domainCount{d, c})
	}
	sort.Slice(sortedDomains, func(i, j int) bool {
		return sortedDomains[i].Count > sortedDomains[j].Count
	})

	var content strings.Builder

	// Stats summary
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Email Statistics</h3>`)
	content.WriteString(`<table class="stats-table">`)
	content.WriteString(fmt.Sprintf(`<tr><td>Total messages</td><td>%d</td></tr>`, stats.Total))
	content.WriteString(fmt.Sprintf(`<tr><td>Inbound (external → local)</td><td>%d</td></tr>`, stats.Inbound))
	content.WriteString(fmt.Sprintf(`<tr><td>Outbound (local → external)</td><td>%d</td></tr>`, stats.Outbound))
	content.WriteString(fmt.Sprintf(`<tr><td>Internal (local → local)</td><td>%d</td></tr>`, stats.Internal))
	content.WriteString(`</table>`)
	content.WriteString(`</div>`)

	// Top domains
	if len(sortedDomains) > 0 {
		content.WriteString(`<div class="card">`)
		content.WriteString(`<h3>External Domains</h3>`)
		content.WriteString(`<table class="stats-table">`)
		for i, dc := range sortedDomains {
			if i >= 10 {
				break
			}
			content.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, dc.Domain, dc.Count))
		}
		content.WriteString(`</table>`)
		content.WriteString(`</div>`)
	}

	// Recent messages
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Recent Messages</h3>`)
	
	if len(messages) == 0 {
		content.WriteString(`<p class="text-muted">No messages yet.</p>`)
	} else {
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Dir</th><th>From</th><th class="hide-mobile">To</th><th>Subject</th></tr>`)

		for _, msg := range messages {
			fromExternal := mail.IsExternalAddress(msg.FromID)
			toExternal := mail.IsExternalAddress(msg.ToID)
			
			dirClass := "dir-int"
			dirLabel := "↔"
			if fromExternal {
				dirClass = "dir-in"
				dirLabel = "←"
			} else if toExternal {
				dirClass = "dir-out"
				dirLabel = "→"
			}

			fromDisplay := msg.From
			if fromDisplay == "" {
				fromDisplay = msg.FromID
			}
			toDisplay := msg.To
			if toDisplay == "" {
				toDisplay = msg.ToID
			}

			content.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td class="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="addr hide-mobile" title="%s">%s</td>
				<td class="subject" title="%s">%s</td>
			</tr>`,
				msg.CreatedAt.Format("Jan 2 15:04"),
				dirClass, dirLabel,
				fromDisplay, truncate(fromDisplay, 25),
				toDisplay, truncate(toDisplay, 25),
				msg.Subject, truncate(msg.Subject, 40),
			))
		}

		content.WriteString(`</table>`)
	}
	content.WriteString(`</div>`)

	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	html := app.RenderHTMLForRequest("Email", "Email activity", content.String(), r)
	w.Write([]byte(html))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
