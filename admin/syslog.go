package admin

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// SysLogHandler shows the in-memory system log page.
func SysLogHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	entries := app.SysLog()

	// JSON API
	if app.WantsJSON(r) {
		type logEntry struct {
			Time    string `json:"time"`
			Package string `json:"package"`
			Message string `json:"message"`
		}
		out := make([]logEntry, len(entries))
		for i, e := range entries {
			out[i] = logEntry{
				Time:    e.Time.Format("15:04:05"),
				Package: e.Package,
				Message: e.Message,
			}
		}
		// Optional filter
		pkg := r.URL.Query().Get("pkg")
		if pkg != "" {
			var filtered []logEntry
			for _, e := range out {
				if e.Package == pkg {
					filtered = append(filtered, e)
				}
			}
			out = filtered
		}
		app.RespondJSON(w, out)
		return
	}

	var content strings.Builder
	content.WriteString(alertsCard())
	content.WriteString(`<div class="card">`)
	content.WriteString(fmt.Sprintf(`<h3>System Log <span class="count">%d</span></h3>`, len(entries)))

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No log entries yet.</p>`)
	} else {
		content.WriteString(`<script>function muToggleSyslog(id){var d=document.getElementById(id);if(d){d.style.display=d.style.display==='none'?'table-row':'none';}}</script>`)
		content.WriteString(`<div class="scroll-x">`)
		content.WriteString(`<table class="email-log" style="width:100%;table-layout:fixed;">`)
		content.WriteString(`<colgroup><col style="width:110px"><col style="width:90px"><col></colgroup>`)
		content.WriteString(`<tr><th>Time</th><th>Package</th><th>Message</th></tr>`)
		for i, e := range entries {
			rowID := fmt.Sprintf("syslog-row-%d", i)
			content.WriteString(fmt.Sprintf(`<tr class="clickable" onclick="muToggleSyslog('%s')" title="Click to expand">
				<td class="nowrap">%s</td>
				<td style="white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">%s</td>
				<td style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">%s</td>
			</tr>
			<tr id="%s" class="d-none">
				<td colspan="3" style="background:#f9f9f9;padding:8px;word-break:break-all;white-space:pre-wrap;font-size:12px;">%s</td>
			</tr>`,
				rowID,
				e.Time.Format("Jan 2 15:04:05"),
				html.EscapeString(e.Package),
				html.EscapeString(truncateMsg(e.Message, 80)),
				rowID,
				html.EscapeString(e.Message),
			))
		}
		content.WriteString(`</table>`)
		content.WriteString(`</div>`)
	}

	content.WriteString(`</div>`)
	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	app.Respond(w, r, app.Response{Title: "System Log", Description: "System Log", HTML: content.String()})
}

// truncateMsg shortens s to at most max characters for display, appending "…" if truncated.
func truncateMsg(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

// alertsCard is the things somebody has to look at, above the ordinary log.
//
// Above it, and kept separately, because the log is a ring of five hundred
// lines — an hour or two on a busy instance, and then the alert that said the
// key store had refused a write is gone. These were being recorded all along
// and were invisible for exactly that reason.
func alertsCard() string {
	alerts := app.Alerts()
	if len(alerts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="card" style="border-color:#e0a3a3">`)
	fmt.Fprintf(&b, `<h3 class="text-error">Alerts <span class="count">%d</span></h3>`, len(alerts))
	b.WriteString(`<p class="text-sm text-muted">Things this instance did that it should ` +
		`not have had to, or refused in order to protect itself. Kept apart from the log ` +
		`below, which rolls over.</p>`)
	b.WriteString(`<div class="scroll-x"><table class="email-log" style="width:100%;table-layout:fixed">`)
	b.WriteString(`<colgroup><col style="width:130px"><col style="width:90px"><col></colgroup>`)
	b.WriteString(`<tr><th>When</th><th>Where</th><th>What</th></tr>`)
	for _, a := range alerts {
		fmt.Fprintf(&b, `<tr><td class="nowrap">%s</td><td>%s</td>`+
			`<td style="word-break:break-word;white-space:pre-wrap">%s</td></tr>`,
			a.Time.Format("Jan 2 15:04:05"),
			html.EscapeString(a.Package),
			html.EscapeString(a.Message))
	}
	b.WriteString(`</table></div></div>`)
	return b.String()
}
