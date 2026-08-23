package admin

// What this instance is watching, and what it would say.
//
// A page rather than only settings, for one reason: an alerting system nobody
// has ever seen fire is an alerting system nobody trusts. The thresholds are
// here beside the values they are compared against, so "am I about to be told
// about this" is answerable by looking rather than by waiting — and there is a
// button that sends one, because the only way to know the delivery works is for
// a message to arrive.
//
// The settings themselves are edited at /admin/config with everything else.
// Two places to change one number is how they come to disagree; this shows
// them and links there.

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/internal/usage"
)

// AlertsHandler serves /admin/alerts.
func AlertsHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	var sb strings.Builder
	sb.WriteString(usage.CSS)
	sb.WriteString(alertsCSS)

	if r.Method == http.MethodPost {
		if !auth.ValidCSRF(r) {
			sb.WriteString(app.Problem("That form was stale. Reload and try again."))
		} else {
			// Straight to raise, through the same cooldown and the same
			// delivery as a real one. A test that took a shortcut would prove
			// the shortcut works.
			raise(alert{
				Key:   "test:" + acc.ID,
				What:  "Test alert",
				Why:   "You pressed the button on /admin/alerts. If this arrived, alerts work.",
				Where: "/admin/alerts",
			})
			sb.WriteString(app.NoteHTML(`Sent to every admin, at their inbox here and ` +
				`as a push notification where one is registered. It goes through the same ` +
				`cooldown as a real alert, so pressing it again within ` +
				strconv.Itoa(number(settings.Get("ALERT_COOLDOWN_MINUTES"), 360)) + ` minutes does nothing.`))
		}
	}

	if !enabled() {
		sb.WriteString(app.Problem("Alerts are off on this instance (ALERTS). Nothing " +
			"below is being watched."))
	}
	if to := admins(); len(to) == 0 {
		sb.WriteString(app.Problem("This instance has no admin account, so there is " +
			"nobody to tell. Set ADMIN to a username."))
	} else {
		sb.WriteString(`<p class="card-desc">Delivered to ` +
			html.EscapeString(strings.Join(to, ", ")) + ` — as mail in the inbox here, ` +
			`and as a push notification on any device that has registered one. Not a ` +
			`separate channel: an instance whose pitch is that an address is enough ` +
			`should not need its own pipe to reach its operator.</p>`)
	}

	// What is watched, what it is now, and what would trip it. The current
	// value beside the threshold is the whole point of the table.
	_, _, diskPercent := app.DiskUsage()
	rows := []struct{ what, now, when, key string }{
		{
			"A new account", "—", "every one",
			"",
		},
		{
			"An account's first tool call", "—", "once per account",
			"",
		},
		{
			"Calls an hour, whole instance",
			usage.HumanCount(usage.TotalOver(usage.Hour, 1)),
			usage.HumanCount(number(settings.Get("ALERT_CALLS_PER_HOUR"), 5000)) + " or more",
			"ALERT_CALLS_PER_HOUR",
		},
		{
			"Calls an hour, one account",
			usage.HumanCount(busiestAccount()),
			usage.HumanCount(number(settings.Get("ALERT_ACCOUNT_CALLS_PER_HOUR"), 1000)) + " or more",
			"ALERT_ACCOUNT_CALLS_PER_HOUR",
		},
		{
			"Disk",
			fmt.Sprintf("%.0f%% full", diskPercent),
			strconv.Itoa(number(settings.Get("ALERT_DISK_PERCENT"), 85)) + "% or more",
			"ALERT_DISK_PERCENT",
		},
	}

	sb.WriteString(`<div class="card"><table class="alerts-table"><thead><tr>` +
		`<th>Watched</th><th>Now</th><th>Tells you at</th><th>Setting</th>` +
		`</tr></thead><tbody>`)
	for _, row := range rows {
		setting := "—"
		if row.key != "" {
			setting = `<code>` + row.key + `</code>`
		}
		sb.WriteString(`<tr><td>` + html.EscapeString(row.what) + `</td>` +
			`<td class="alerts-now">` + html.EscapeString(row.now) + `</td>` +
			`<td>` + html.EscapeString(row.when) + `</td>` +
			`<td>` + setting + `</td></tr>`)
	}
	sb.WriteString(`</tbody></table>`)
	sb.WriteString(`<p class="card-desc">Set a threshold to <code>0</code> to stop ` +
		`watching that one; set <code>ALERTS</code> to <code>off</code> to stop all of ` +
		`them. Anything that trips stays quiet for ` +
		strconv.Itoa(number(settings.Get("ALERT_COOLDOWN_MINUTES"), 360)) + ` minutes afterwards ` +
		`(<code>ALERT_COOLDOWN_MINUTES</code>) — which is what makes a threshold safe ` +
		`to set low: crossing one costs a message, not a message every five minutes ` +
		`until it is fixed. Edit them at ` + app.TextLink("config", "/admin/config") + `.</p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<form method="post" action="/admin/alerts" class="alerts-test">` +
		`<input type="hidden" name="csrf_token" value="` +
		html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button type="submit">Send a test alert</button></form>`)

	app.Respond(w, r, app.Response{
		Title:       "Alerts",
		Description: "What this instance will tell you about, and where",
		HTML:        sb.String(),
	})
}

// busiestAccount is the most any one account has called in the last hour.
//
// The number the per-account threshold is actually compared against, so the row
// on the page says something true rather than a total nobody is watching.
func busiestAccount() int {
	most := 0
	for _, acc := range auth.AllAccounts() {
		if n := usage.TotalForOver(acc.ID, usage.Hour, 1); n > most {
			most = n
		}
	}
	return most
}

const alertsCSS = `<style>
.alerts-table { width: 100%; border-collapse: collapse; font-size: 14px }
.alerts-table th, .alerts-table td {
  text-align: left;
  padding: 9px 12px 9px 0;
  border-bottom: var(--border-width, 1px) solid var(--card-border, #f0f0f0);
}
.alerts-table th { font-weight: 500; color: var(--text-muted, #888); font-size: 12px }
.alerts-table tr:last-child td { border-bottom: 0 }
.alerts-now { font-variant-numeric: tabular-nums; font-weight: 500 }
.alerts-test { margin-top: 16px }
</style>`
