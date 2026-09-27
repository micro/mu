package admin

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
	"net/http"
	"sort"
	"strings"
)

func ErrorsHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireAdmin(r); err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}
	activity := r.URL.Path == "/admin/activity"
	title, description := "Errors", "Failures that need attention, grouped by recurring problem."
	content := alertsCard()
	if activity {
		title, description = "Activity", "Who used Micro, what they requested and how it went."
		content = ""
	}
	content = `<p class="text-muted">` + description + `</p>` + content + activityLogCard(r, activity)
	app.Respond(w, r, app.Response{Title: title, HTML: `<div class="page-stack log-tables">` + content + `</div>`})
}

func activityLogCard(r *http.Request, activity bool) string {
	entries := usage.Activities(!activity)
	account, surface := strings.TrimSpace(r.FormValue("account")), strings.TrimSpace(r.FormValue("surface"))
	filtered := []usage.Activity{}
	for _, e := range entries {
		if account != "" && e.Account != account || surface != "" && e.Surface != surface {
			continue
		}
		filtered = append(filtered, e)
	}
	var b strings.Builder
	b.WriteString(`<p>Anonymous missing-page requests stay in Activity. <a href="/admin/traffic">View usage totals</a>.</p><details class="disclosure"><summary>About these records</summary><p>Up to 5,000 activity records and 2,000 failures, retained for 14 days; busy periods may cover less time. Recording began with the activity update. Historical totals cannot reconstruct individual requests.</p></details>`)
	target := "/admin/errors"
	if activity {
		target = "/admin/activity"
	}

	fmt.Fprintf(&b, `<form class="search-bar" method="POST" action="%s">%s<input name="account" aria-label="Username" placeholder="Username (exact)" value="%s"><select name="surface" aria-label="Source"><option value="">All sources</option>`, target, app.CSRFField(auth.CSRFToken(r)), html.EscapeString(account))
	for _, s := range []string{"mcp", "api", "agent", "http", "provider", "credentials"} {
		selected := ""
		if s == surface {
			selected = " selected"
		}
		fmt.Fprintf(&b, `<option%s>%s</option>`, selected, s)
	}
	b.WriteString(`</select><button type="submit">Filter</button></form>`)
	// Group failures to make recurring issues visible before individual records.
	if !activity {
		counts := map[string]int{}
		priority := map[string]int{}
		for _, e := range filtered {
			outcome := e.Outcome
			if e.Status >= 400 && outcome == "request failed" {
				outcome = usage.FailureKind(e.Status, "")
			}
			key := outcome + " · " + e.Surface + " · " + e.Operation
			if e.Status > 0 {
				key += fmt.Sprintf(" · HTTP %d", e.Status)
			}
			counts[key]++
			score := 1
			if e.Account != "guest" {
				score = 2
			}
			if e.Outcome == "credits or quota" || e.Outcome == "timeout" || e.Outcome == "service failure" {
				score = 3
			}
			if score > priority[key] {
				priority[key] = score
			}
		}
		keys := []string{}
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if priority[keys[i]] != priority[keys[j]] {
				return priority[keys[i]] > priority[keys[j]]
			}
			return counts[keys[i]] > counts[keys[j]]
		})
		b.WriteString(`<h2>Recurring issues</h2><ul>`)
		for i, k := range keys {
			if i == 10 {
				break
			}
			fmt.Fprintf(&b, "<li>%s — %d</li>", html.EscapeString(k), counts[k])
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<div class="table-scroll"><table class="data-table"><thead><tr><th>Time (UTC)</th><th>Username</th><th>Source</th><th>Operation</th><th>Outcome</th><th>HTTP</th><th>Duration</th><th>Token ID</th></tr></thead><tbody>`)
	for i, e := range filtered {
		if i == 500 {
			break
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d ms</td><td>%s</td></tr>`, e.At.UTC().Format("Jan 2 15:04:05"), html.EscapeString(e.Account), html.EscapeString(e.Surface), html.EscapeString(e.Operation), html.EscapeString(e.Outcome), e.Status, e.DurationMS, html.EscapeString(e.TokenID))
	}
	if len(filtered) == 0 {
		b.WriteString(`<tr><td colspan="8">No matching records.</td></tr>`)
	}
	b.WriteString(`</tbody></table></div><p>Showing at most 500 matching records. Provider failures may have no account attribution. A failed request is not, by itself, evidence of abuse.</p>`)
	return b.String()
}
