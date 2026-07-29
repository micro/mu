package admin

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
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
	relays := mail.RecentRelays(50)
	relaySent, relayFailed := mail.RelayStats()

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
	content.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">Counts stored messages. External relays (including system mail) are logged separately below: %d delivered, %d failed.</p>`, relaySent, relayFailed))
	content.WriteString(`</div>`)

	// Outbound relay log — every message handed to an external MX, including
	// system mail (calendar invites, verification) that is never stored as a
	// user message and so does not appear in Recent Messages.
	content.WriteString(`<div class="card">`)
	content.WriteString(`<h3>Outbound Relay</h3>`)
	content.WriteString(`<p class="text-muted text-sm">Everything sent to an external mail server, including system mail. Last ` + fmt.Sprintf("%d", len(relays)) + `.</p>`)

	if len(relays) == 0 {
		content.WriteString(`<p class="text-muted">Nothing relayed yet.</p>`)
	} else {
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Status</th><th class="hide-mobile">From</th><th>To</th><th>Subject</th></tr>`)
		for _, e := range relays {
			statusClass, statusLabel := "dir-out", "→"
			if !e.OK {
				statusClass, statusLabel = "dir-in", "✗"
			}
			title := e.MessageID
			if e.Error != "" {
				title = e.Error
			}
			content.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td class="%s" title="%s">%s</td>
				<td class="addr hide-mobile" title="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="subject" title="%s">%s</td>
			</tr>`,
				e.Time.Format("Jan 2 15:04"),
				statusClass, html.EscapeString(title), statusLabel,
				html.EscapeString(e.From), html.EscapeString(truncate(e.From, 25)),
				html.EscapeString(e.To), html.EscapeString(truncate(e.To, 25)),
				html.EscapeString(e.Subject), html.EscapeString(truncate(e.Subject, 40)),
			))
		}
		content.WriteString(`</table>`)
	}
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
	content.WriteString(`<p class="text-muted text-sm">Click a row to expand details.</p>`)

	if len(messages) == 0 {
		content.WriteString(`<p class="text-muted">No messages yet.</p>`)
	} else {
		content.WriteString(`<table class="email-log">`)
		content.WriteString(`<tr><th>Time</th><th>Dir</th><th>From</th><th>To</th><th>Subject</th></tr>`)

		for i, msg := range messages {
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

			// Build cell text and tooltip for From
			fromCell, fromTitle := addrDisplay(msg.From, msg.FromID, fromExternal)
			// Build cell text and tooltip for To
			toCell, toTitle := addrDisplay(msg.To, msg.ToID, toExternal)

			rowID := fmt.Sprintf("erow%d", i)
			content.WriteString(fmt.Sprintf(`<tr class="clickable" data-detail="%s" onclick="var d=document.getElementById(this.dataset.detail);d.style.display=d.style.display===''?'none':'';">
				<td>%s</td>
				<td class="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="addr" title="%s">%s</td>
				<td class="subject" title="%s">%s</td>
			</tr>
			<tr id="%s" style="display:none"><td colspan="5" class="email-detail">
				<strong>From:</strong> %s &nbsp;
				<strong>To:</strong> %s &nbsp;
				<strong>Subject:</strong> %s<br>
				<strong>ID:</strong> <code>%s</code>
			</td></tr>`,
				rowID,
				msg.CreatedAt.Format("Jan 2 15:04"),
				dirClass, dirLabel,
				html.EscapeString(fromTitle), html.EscapeString(truncate(fromCell, 25)),
				html.EscapeString(toTitle), html.EscapeString(truncate(toCell, 25)),
				html.EscapeString(msg.Subject), html.EscapeString(truncate(msg.Subject, 40)),
				rowID,
				html.EscapeString(fromTitle), html.EscapeString(toTitle), html.EscapeString(msg.Subject),
				html.EscapeString(msg.ID),
			))
		}

		content.WriteString(`</table>`)
	}
	content.WriteString(`</div>`)

	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	page := app.RenderHTMLForRequest("Email", "Email activity", content.String(), r)
	w.Write([]byte(page))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// addrDisplay returns a short cell label and a full tooltip for a message address.
// name is the display name (may be empty), id is the account ID or email address,
// and external indicates whether the address is an external email (contains @).
// For external addresses the cell shows the email and the tooltip is "Name <email>".
// For internal users the cell shows "@username" and the tooltip is "Name (@username)".
func addrDisplay(name, id string, external bool) (cell, title string) {
	if external {
		cell = id
		if name != "" && name != id {
			title = name + " <" + id + ">"
		} else {
			title = id
		}
	} else {
		cell = "@" + id
		if name != "" && name != id {
			title = name + " (@" + id + ")"
		} else {
			title = "@" + id
		}
	}
	return
}
