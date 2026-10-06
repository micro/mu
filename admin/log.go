package admin

// What this instance has been saying, in one place.
//
// There were three pages and they were the same page: /admin/log printed the
// lines this process wrote, /admin/api printed the calls it made to somebody
// else, /admin/email printed what the mail server did — and the only way to
// correlate "the news service logged an error" with "the feed returned 502",
// or "the alert was raised" with "the relay failed", was to open them in three
// tabs and compare timestamps. Three nav entries for one question — what
// happened just now — and the answer split across them.
//
// One page, three log sources. Request activity and grouped failures have
// their own admin pages; they are not substitutes for these technical logs.
//
// The mail *rules* did not come here. What may be sent and who is refused is a
// setting rather than an event, and it is on /admin/spam with the filter.

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

	tab := r.URL.Query().Get("tab")
	api, mailTab := tab == "api", tab == "mail"

	// JSON is the system log only. Nothing asks for the API calls that way, and
	// a shape that changes with a query parameter is not an API.
	if app.WantsJSON(r) && !api {
		respondSysLogJSON(w, r)
		return
	}

	// Retain links from the previous combined view without mixing activity
	// records into the system logs.
	if tab == "activity" || tab == "errors" {
		destination := "/admin/activity"
		if tab == "errors" {
			destination = "/admin/errors"
		}
		http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
		return
	}
	var content strings.Builder
	content.WriteString(`<p class="text-muted">Technical logs from the server, external calls and mail delivery.</p>`)
	content.WriteString(technicalLogTabs(tab))
	if api {
		content.WriteString(apiLogCard())
	} else if mailTab {
		content.WriteString(mailLogCard())
	} else {
		content.WriteString(sysLogCard())
	}
	app.Respond(w, r, app.Response{Title: "Logs", HTML: `<div class="page-stack log-tables">` + content.String() + `</div>`})
}

func MailLogMoved(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/log?tab=mail", http.StatusSeeOther)
}

func technicalLogTabs(on string) string {
	return `<nav class="form-actions" aria-label="Technical log sources">` +
		app.PillLink("System", "/admin/log", on != "api" && on != "mail") +
		app.PillLink("External calls", "/admin/log?tab=api", on == "api") +
		app.PillLink("Mail", "/admin/log?tab=mail", on == "mail") + `</nav>`
}

// sysLogCard is the in-memory ring of lines this process wrote.
func sysLogCard() string {
	entries := app.SysLog()

	var content strings.Builder
	// No heading. The page is titled "System Log" and the selected tab says
	// System, so a third copy of the same two words is the only thing above
	// the table. It also carried a `<span class="count">` — a class defined
	// nowhere but under .admin-links, so it rendered as a bare number stuck to
	// the end of the title, and the number was the ring buffer's capacity
	// rather than anything about this instance.
	content.WriteString(`<div class="card">`)

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No log entries yet.</p></div>`)
		return content.String()
	}

	content.WriteString(`<div class="table-scroll" tabindex="0" role="region" aria-label="Log and usage records">`)
	content.WriteString(`<table class="data-table table-wide">`)
	content.WriteString(`<colgroup><col class="w-110"><col class="w-90"><col></colgroup>`)
	content.WriteString(`<tr><th>Time</th><th>Package</th><th>Message</th></tr>`)
	for _, e := range entries {
		fmt.Fprintf(&content, `<tr><td>%s</td><td>%s</td><td><details class="disclosure"><summary>%s</summary><pre>%s</pre></details></td></tr>`,
			e.Time.Format("Jan 2 15:04:05"), html.EscapeString(e.Package),
			html.EscapeString(truncateMsg(e.Message, 80)), html.EscapeString(e.Message))
	}
	content.WriteString(`</table></div></div>`)
	return content.String()
}

// apiLogCard is the calls this instance made to somebody else.
func apiLogCard() string {
	entries := app.APILog()

	var content strings.Builder
	content.WriteString(`<div class="card">`)

	if len(entries) == 0 {
		content.WriteString(`<p class="text-muted">No API calls recorded yet.</p></div>`)
		return content.String()
	}

	content.WriteString(`<div class="table-scroll" tabindex="0" role="region" aria-label="Log and usage records"><table class="data-table table-wide">`)
	content.WriteString(`<tr><th>Time</th><th>Service</th><th>Method</th><th>URL</th><th>Status</th><th>Duration</th><th>Error</th></tr>`)

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

		if e.Kind == "model" {
			statusLabel = html.EscapeString(e.Outcome)
			if statusLabel == "done" {
				statusClass = "dir-in"
			}
		}

		if e.Error != "" {
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
			<td class="addr" title="%s">%s</td>
			<td class="%s">%s</td>
			<td>%dms</td>
			<td class="subject" title="%s">%s</td>
		</tr>`,
			e.Time.Format("Jan 2 15:04:05"),
			html.EscapeString(e.Service),
			html.EscapeString(e.Method),
			html.EscapeString(e.URL), html.EscapeString(truncate(e.URL, 50)),
			statusClass, statusLabel,
			e.Duration.Milliseconds(),
			html.EscapeString(e.Error), html.EscapeString(errStr),
		))

		if e.Error != "" {
			fmt.Fprintf(&content, `<tr><td colspan="7"><details class="disclosure"><summary>Error details</summary><pre class="raw-sm">%s</pre></details></td></tr>`, html.EscapeString(e.Error))
		}
		if e.Kind == "model" {
			fmt.Fprintf(&content, `<tr><td colspan="7">Model: %s · Run: %s · Attempt: %d · Tokens: %d in / %d out</td></tr>`, html.EscapeString(e.Model), html.EscapeString(e.RunID), e.Attempt, e.InputTokens, e.OutputTokens)
		}
		if e.Kind == "image" {
			fmt.Fprintf(&content, `<tr><td colspan="7">Model: %s · Outcome: %s</td></tr>`, html.EscapeString(e.Model), html.EscapeString(e.Outcome))
		}
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
	b.WriteString(`<div class="table-scroll" tabindex="0" role="region" aria-label="Log and usage records"><table class="data-table table-wide">`)
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
