package admin

import (
	"fmt"
	"strings"
	"time"

	"mu/internal/app"
)

// spendCard is what this instance has paid third parties: a total, a breakdown
// by service, and the recent calls behind it.
//
// The other half of /admin/traffic — see the tabs there for why the two are
// one page.
func spendCard() string {
	summary := app.GetUsageSummary()
	uptime := time.Since(summary.Since).Round(time.Minute)

	var sb strings.Builder
	sb.WriteString(`<style>
.ai-usage-table { width:100%; border-collapse:collapse; font-size:0.85rem; }
.ai-usage-table th, .ai-usage-table td { padding:6px 8px; white-space:nowrap; }
.ai-usage-table td:first-child { word-break:break-all; white-space:normal; }
@media (max-width: 600px) {
  .ai-usage-table { font-size:0.72rem; }
  .ai-usage-table th, .ai-usage-table td { padding:4px 3px; }
}
</style>`)
	sb.WriteString(fmt.Sprintf(`<p>Tracking since %s (%s ago)</p>`, summary.Since.Format("2006-01-02 15:04"), uptime))
	sb.WriteString(fmt.Sprintf(`<p><strong>Total: %d calls, est $%.4f</strong></p>`, summary.TotalCalls, summary.TotalCost/100))

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
