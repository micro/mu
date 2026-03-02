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
		content.WriteString(`<script>function muToggleSyslog(id){var d=document.getElementById(id);if(d){d.style.display=d.style.display==='none'?'table-row':'none';}}</script>`)
		content.WriteString(`<div style="overflow-x:auto;">`)
		content.WriteString(`<table class="email-log" style="width:100%;table-layout:fixed;">`)
		content.WriteString(`<colgroup><col style="width:110px"><col style="width:90px"><col></colgroup>`)
		content.WriteString(`<tr><th>Time</th><th>Package</th><th>Message</th></tr>`)
		for i, e := range entries {
			rowID := fmt.Sprintf("syslog-row-%d", i)
			content.WriteString(fmt.Sprintf(`<tr style="cursor:pointer;" onclick="muToggleSyslog('%s')" title="Click to expand">
				<td style="white-space:nowrap;">%s</td>
				<td style="white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">%s</td>
				<td style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">%s</td>
			</tr>
			<tr id="%s" style="display:none;">
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

	pageHTML := app.RenderHTMLForRequest("System Log", "System Log", content.String(), r)
	w.Write([]byte(pageHTML))
}

// truncateMsg shortens s to at most max characters for display, appending "…" if truncated.
func truncateMsg(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}
