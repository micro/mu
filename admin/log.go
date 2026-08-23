package admin

// What this instance has been saying, in one place.
//
// There were two pages and they were the same page: /admin/log printed the
// lines this process wrote, /admin/api printed the calls it made to somebody
// else, and the only way to correlate "the news service logged an error" with
// "the feed returned 502" was to open both in two tabs and compare
// timestamps. Two nav entries for one question — what happened just now — and
// the answer split across them.
//
// So: one page, two tabs. Alerts stay above both, because an alert is a thing
// to look at whichever log you came for.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// LogHandler shows the system log and the external API calls.
func LogHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	api := r.URL.Query().Get("tab") == "api"

	// JSON is the system log only. Nothing asks for the API calls that way, and
	// a shape that changes with a query parameter is not an API.
	if app.WantsJSON(r) && !api {
		respondSysLogJSON(w, r)
		return
	}

	var content strings.Builder
	content.WriteString(alertsCard())
	content.WriteString(logTabs(api))
	if api {
		content.WriteString(apiLogCard())
	} else {
		content.WriteString(sysLogCard())
	}
	content.WriteString(`<p><a href="/admin">← Back to Admin</a></p>`)

	title := "System Log"
	if api {
		title = "API Log"
	}
	app.Respond(w, r, app.Response{Title: title, Description: "Logs", HTML: content.String()})
}

// logTabs is the switch between the two.
func logTabs(api bool) string {
	tab := func(href, label string, on bool) string {
		class := "pill"
		if on {
			class = "pill pill-on"
		}
		return `<a class="` + class + `" href="` + href + `">` + label + `</a>`
	}
	return `<div class="d-flex gap-2 mb-3">` +
		tab("/admin/log", "System", !api) +
		tab("/admin/log?tab=api", "External calls", api) +
		`</div>`
}

// sysLogCard is the in-memory ring of lines this process wrote.
func sysLogCard() string {
	entries := app.SysLog()

	var content strings.Builder
	content.WriteString(`<div class="card">`)
	content.WriteString(fmt.Sprintf(`<h3>System Log <span class="count">%d</span></h3>`, len(entries)))

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No log entries yet.</p></div>`)
		return content.String()
	}

	content.WriteString(`<script>function muToggleSyslog(id){var d=document.getElementById(id);if(d){d.style.display=d.style.display==='none'?'table-row':'none';}}</script>`)
	content.WriteString(`<div class="scroll-x">`)
	content.WriteString(`<table class="email-log fixed-table">`)
	content.WriteString(`<colgroup><col class="w-110"><col class="w-90"><col></colgroup>`)
	content.WriteString(`<tr><th>Time</th><th>Package</th><th>Message</th></tr>`)
	for i, e := range entries {
		rowID := fmt.Sprintf("syslog-row-%d", i)
		content.WriteString(fmt.Sprintf(`<tr class="clickable" onclick="muToggleSyslog('%s')" title="Click to expand">
			<td class="nowrap">%s</td>
			<td class="ellipsis">%s</td>
			<td class="ellipsis">%s</td>
		</tr>
		<tr id="%s" class="d-none">
			<td colspan="3" class="raw">%s</td>
		</tr>`,
			rowID,
			e.Time.Format("Jan 2 15:04:05"),
			html.EscapeString(e.Package),
			html.EscapeString(truncateMsg(e.Message, 80)),
			rowID,
			html.EscapeString(e.Message),
		))
	}
	content.WriteString(`</table></div></div>`)
	return content.String()
}

// apiLogCard is the calls this instance made to somebody else.
func apiLogCard() string {
	entries := app.APILog()

	var content strings.Builder
	content.WriteString(`<div class="card">`)
	content.WriteString(fmt.Sprintf(`<h3>External API Calls <span class="count">%d</span></h3>`, len(entries)))

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No API calls recorded yet.</p></div>`)
		return content.String()
	}

	content.WriteString(`<div class="scroll-x"><table class="email-log">`)
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

	content.WriteString(`</table></div></div>`)
	return content.String()
}

// respondSysLogJSON answers ?pkg= for whatever reads the log over HTTP.
func respondSysLogJSON(w http.ResponseWriter, r *http.Request) {
	type logEntry struct {
		Time    string `json:"time"`
		Package string `json:"package"`
		Message string `json:"message"`
	}
	entries := app.SysLog()
	out := make([]logEntry, 0, len(entries))
	pkg := r.URL.Query().Get("pkg")
	for _, e := range entries {
		if pkg != "" && e.Package != pkg {
			continue
		}
		out = append(out, logEntry{
			Time:    e.Time.Format("15:04:05"),
			Package: e.Package,
			Message: e.Message,
		})
	}
	app.RespondJSON(w, out)
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
	b.WriteString(`<div class="card border-bad">`)
	fmt.Fprintf(&b, `<h3 class="text-error">Alerts <span class="count">%d</span></h3>`, len(alerts))
	b.WriteString(`<p class="text-sm text-muted">Things this instance did that it should ` +
		`not have had to, or refused in order to protect itself. Kept apart from the log ` +
		`below, which rolls over.</p>`)
	b.WriteString(`<div class="scroll-x"><table class="email-log fixed-table">`)
	b.WriteString(`<colgroup><col class="w-130"><col class="w-90"><col></colgroup>`)
	b.WriteString(`<tr><th>When</th><th>Where</th><th>What</th></tr>`)
	for _, a := range alerts {
		fmt.Fprintf(&b, `<tr><td class="nowrap">%s</td><td>%s</td>`+
			`<td class="wrap-anywhere">%s</td></tr>`,
			a.Time.Format("Jan 2 15:04:05"),
			html.EscapeString(a.Package),
			html.EscapeString(a.Message))
	}
	b.WriteString(`</table></div></div>`)
	return b.String()
}
