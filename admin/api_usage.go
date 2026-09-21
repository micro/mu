package admin

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"mu/account"
	"mu/internal/app"
	"mu/internal/quota"
)

// spendCard is what this instance has paid third parties: a total, a breakdown
// by service, and the recent calls behind it.
//
// The other half of /admin/traffic — see the tabs there for why the two are
// one page.
func spendCard() string {
	summary := app.GetUsageSummary()
	since, days := app.DailyCosts()

	var sb strings.Builder
	sb.WriteString(`<h3>Included usage today</h3>`)
	fmt.Fprintf(&sb, `<p>%d credits used across accounts. `, account.IncludedUsage())
	if cap := quota.DailyPoolCredits(); cap > 0 {
		fmt.Fprintf(&sb, `Shared daily limit: %d credits.`, cap)
	} else {
		sb.WriteString(`No shared daily limit is configured.`)
	}
	sb.WriteString(` Renews at 00:00 UTC. Funded credit remains available.</p><p class="text-sm text-muted">Set DAILY_POOL_CREDITS or daily_pool_credits in quota.json. This limits free product credit, not provider invoices. Provider estimates below include paid and operator use too.</p>`)
	fmt.Fprintf(&sb, `<h3>Daily provider estimates</h3><p class="text-sm text-muted">Collected since %s UTC; the first day may be partial. Up to 90 days. Estimates are not invoices; recent activity may take a few seconds to save.</p>`, since.UTC().Format("2006-01-02 15:04"))
	sb.WriteString(`<div class="scroll-x"><table><thead><tr><th>UTC day</th><th>Calls</th><th>Estimated cost</th></tr></thead><tbody>`)
	for _, d := range days {
		fmt.Fprintf(&sb, `<tr><td>%s</td><td>%d</td><td>$%.4f</td></tr>`, d.Day, d.Calls, d.CostCents/100)
	}
	sb.WriteString(`</tbody></table></div><h3>Model cost by account</h3><p class="text-sm text-muted">Last 30 UTC days of attributed model estimates, including failed runs. Collection starts with this release. Excludes unattributed calls, tools, hosting and payment fees.</p><div class="scroll-x"><table><thead><tr><th>Account</th><th>Model records</th><th>Estimated cost</th></tr></thead><tbody>`)
	costs := app.AccountCosts()
	ids := make([]string, 0, len(costs))
	for id := range costs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if costs[ids[i]].CostCents == costs[ids[j]].CostCents {
			return ids[i] < ids[j]
		}
		return costs[ids[i]].CostCents > costs[ids[j]].CostCents
	})
	for _, id := range ids {
		d := costs[id]
		fmt.Fprintf(&sb, `<tr><td>%s</td><td>%d</td><td>$%.4f</td></tr>`, html.EscapeString(id), d.Calls, d.CostCents/100)
	}
	if len(ids) == 0 {
		sb.WriteString(`<tr><td colspan="3">No attributed model costs yet.</td></tr>`)
	}
	sb.WriteString(`</tbody></table></div><h3>Recent recorded activity</h3><p class="text-sm text-muted">The breakdown below covers only the latest 2,000 recorded calls.</p>`)

	// Usage by service table
	sb.WriteString(`<h3>By Service</h3>`)
	sb.WriteString(`<div class="scroll-x"><table class="ai-usage-table"><thead><tr>
		<th>Service</th><th>Calls</th><th>Cost</th>
	</tr></thead><tbody>`)

	for _, su := range summary.ByService {
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td><td>$%.4f</td></tr>`,
			su.Service, su.Calls, su.CostCents/100))
	}

	sb.WriteString(`</tbody></table></div>`)

	// Recent calls
	sb.WriteString(`<h3>Recent Calls</h3>`)
	sb.WriteString(`<div class="scroll-x"><table class="ai-usage-table"><thead><tr>
		<th>Time</th><th>Service</th><th>Caller</th><th>Detail</th><th>Cost</th>
	</tr></thead><tbody>`)

	for _, r := range summary.RecentCalls {
		detail := formatDetail(r)
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>$%.4f</td></tr>`,
			r.Timestamp.Format("15:04:05"), r.Service, r.Caller, detail, r.CostCents/100))
	}

	sb.WriteString(`</tbody></table></div>`)

	return sb.String()
}

// formatDetail renders service-specific details into a short string.
func formatDetail(r app.UsageRecord) string {
	if r.Details == nil {
		return ""
	}
	switch r.Service {
	case "claude":
		model, _ := r.Details["model"].(string)
		inTok := intDetail(r.Details, "input_tokens")
		outTok := intDetail(r.Details, "output_tokens")
		cacheR := intDetail(r.Details, "cache_read_tokens")
		cacheW := intDetail(r.Details, "cache_creation_tokens")
		s := fmt.Sprintf("%s in:%d out:%d", model, inTok, outTok)
		if cacheR > 0 {
			s += fmt.Sprintf(" cr:%d", cacheR)
		}
		if cacheW > 0 {
			s += fmt.Sprintf(" cw:%d", cacheW)
		}
		return s
	default:
		// Generic: show all details
		var parts []string
		for k, v := range r.Details {
			parts = append(parts, fmt.Sprintf("%s:%v", k, v))
		}
		return strings.Join(parts, " ")
	}
}

func intDetail(d map[string]any, key string) int {
	v, ok := d[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
