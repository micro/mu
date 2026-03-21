package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
)

// AIUsageHandler shows Claude API usage breakdown by caller
func AIUsageHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	summary := ai.GetUsageSummary()
	uptime := time.Since(summary.Since).Round(time.Minute)

	var sb strings.Builder
	sb.WriteString(`<h2>Claude API Usage</h2>`)
	sb.WriteString(fmt.Sprintf(`<p>Tracking since %s (%s ago)</p>`, summary.Since.Format("2006-01-02 15:04"), uptime))
	sb.WriteString(fmt.Sprintf(`<p><strong>Total: %d calls, est $%.4f</strong></p>`, summary.TotalCalls, summary.TotalCost/100))

	// Usage by caller table
	sb.WriteString(`<h3>Usage by Caller</h3>`)
	sb.WriteString(`<table><thead><tr>
		<th>Caller</th><th>Calls</th><th>Input Tokens</th><th>Output Tokens</th>
		<th>Cache Read</th><th>Cache Write</th><th>Est Cost</th>
	</tr></thead><tbody>`)

	for _, cu := range summary.ByCaller {
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>$%.4f</td></tr>`,
			cu.Caller, cu.Calls, cu.InputTokens, cu.OutputTokens,
			cu.CacheReadTokens, cu.CacheCreationTokens, cu.TotalCostCents/100))
	}

	sb.WriteString(`</tbody></table>`)

	// Recent calls
	sb.WriteString(`<h3>Recent Calls</h3>`)
	sb.WriteString(`<table><thead><tr>
		<th>Time</th><th>Caller</th><th>Model</th><th>In</th><th>Out</th><th>Cache</th><th>Cost</th>
	</tr></thead><tbody>`)

	for _, r := range summary.RecentCalls {
		cache := ""
		if r.CacheReadTokens > 0 {
			cache = fmt.Sprintf("read:%d", r.CacheReadTokens)
		} else if r.CacheCreationTokens > 0 {
			cache = fmt.Sprintf("write:%d", r.CacheCreationTokens)
		}
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%s</td><td>$%.4f</td></tr>`,
			r.Timestamp.Format("15:04:05"), r.Caller, r.Model,
			r.InputTokens, r.OutputTokens, cache, r.EstimatedCostCents/100))
	}

	sb.WriteString(`</tbody></table>`)

	html := app.RenderHTMLForRequest("Admin", "AI Usage", sb.String(), r)
	w.Write([]byte(html))
}
