package admin

// The operator's answer to "what is this instance doing?".
//
// /admin/usage is money out — what Mu spends on third-party APIs. This is the
// other direction: what people and agents are asking of it, over time and
// broken down by endpoint, tool, caller and surface. Both are usage; only one
// of them was here.
//
// The chart is a server-rendered SVG. A page of bars needs no library, no CDN
// (the CSP would refuse one anyway) and no client-side fetching, and it is the
// same data whether the operator is on a laptop or a phone.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

// window is one of the three views the page offers.
type window struct {
	slug   string
	label  string
	res    usage.Resolution
	points int
	format string // how a bucket's time is labelled
}

var windows = []window{
	{"live", "Last 2 hours", usage.Minute, 120, "15:04"},
	{"week", "Last 7 days", usage.Hour, 24 * 7, "Mon 15:00"},
	{"quarter", "Last 90 days", usage.Day, 90, "2 Jan"},
}

// TrafficHandler shows what this instance is being asked to do.
func TrafficHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	win := windows[0]
	if q := r.URL.Query().Get("window"); q != "" {
		for _, c := range windows {
			if c.slug == q {
				win = c
			}
		}
	}

	series := usage.Series(win.res, win.points)

	var sb strings.Builder
	sb.WriteString(trafficCSS)

	// Headline numbers, so the first thing on the page is the answer to "how
	// busy is it".
	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	stat(&sb, "Last hour", usage.TotalOver(usage.Minute, 60))
	stat(&sb, "Last 24 hours", usage.TotalOver(usage.Hour, 24))
	stat(&sb, "Last 7 days", usage.TotalOver(usage.Hour, 24*7))
	stat(&sb, "Last 90 days", usage.TotalOver(usage.Day, 90))
	sb.WriteString(`</div></div>`)

	// Chart.
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<div class="traffic-tabs">`)
	for _, c := range windows {
		active := ""
		if c.slug == win.slug {
			active = " active"
		}
		fmt.Fprintf(&sb, `<a href="/admin/traffic?window=%s" class="traffic-tab%s">%s</a>`,
			c.slug, active, html.EscapeString(c.label))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(chartSVG(series, win))
	sb.WriteString(`</div>`)

	// Breakdowns over the same window.
	sb.WriteString(`<div class="traffic-grid">`)
	table(&sb, "Endpoints and tools", usage.Top(win.res, win.points, usage.ByName, 20))
	table(&sb, "Callers", usage.Top(win.res, win.points, usage.ByUser, 20))
	table(&sb, "Surface", usage.Top(win.res, win.points, usage.BySurface, 10))
	sb.WriteString(`</div>`)

	sb.WriteString(`<p class="text-sm text-muted">Counts only — no request is stored. ` +
		`Minutes are kept for 2 hours, hours for 7 days, days for 90.</p>`)

	w.Write([]byte(app.RenderHTMLForRequest("Traffic", "What this instance is being asked to do", sb.String(), r)))
}

func stat(sb *strings.Builder, label string, n int) {
	fmt.Fprintf(sb, `<div class="traffic-stat"><span class="traffic-stat-n">%s</span><span class="traffic-stat-l">%s</span></div>`,
		humanCount(n), html.EscapeString(label))
}

// humanCount keeps a big number readable in a small tile.
func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

// chartSVG draws the series as bars. Plain SVG, sized in percentages so it
// scales to the width it is given.
func chartSVG(series []usage.Bucket, win window) string {
	const height = 160

	peak := 0
	total := 0
	for _, b := range series {
		total += b.Total
		if b.Total > peak {
			peak = b.Total
		}
	}
	if peak == 0 {
		return `<p class="text-sm text-muted">Nothing recorded in this window yet.</p>`
	}

	width := len(series) * 8
	if width < 240 {
		width = 240
	}
	step := float64(width) / float64(len(series))

	var sb strings.Builder
	fmt.Fprintf(&sb, `<div class="traffic-chart"><svg viewBox="0 0 %d %d" preserveAspectRatio="none" role="img" aria-label="%s, peak %d">`,
		width, height, html.EscapeString(win.label), peak)

	for i, b := range series {
		if b.Total == 0 {
			continue
		}
		h := float64(b.Total) / float64(peak) * float64(height-2)
		if h < 1 {
			h = 1
		}
		x := float64(i) * step
		// A title makes every bar readable on hover without a line of
		// JavaScript.
		fmt.Fprintf(&sb, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" class="traffic-bar"><title>%s — %d</title></rect>`,
			x, float64(height)-h, step*0.8, h,
			html.EscapeString(b.At.Local().Format(win.format)), b.Total)
	}
	sb.WriteString(`</svg></div>`)

	// Axis: first, middle and last labels. More than that is unreadable at
	// phone width and tells the operator nothing extra.
	first := series[0].At.Local().Format(win.format)
	mid := series[len(series)/2].At.Local().Format(win.format)
	last := series[len(series)-1].At.Local().Format(win.format)
	fmt.Fprintf(&sb, `<div class="traffic-axis"><span>%s</span><span>%s</span><span>%s</span></div>`,
		html.EscapeString(first), html.EscapeString(mid), html.EscapeString(last))
	fmt.Fprintf(&sb, `<p class="text-sm text-muted">%d calls in this window · peak %d per %s</p>`,
		total, peak, strings.TrimSuffix(string(win.res), "s"))
	return sb.String()
}

func table(sb *strings.Builder, title string, rows []usage.Count) {
	sb.WriteString(`<div class="card"><h3>` + html.EscapeString(title) + `</h3>`)
	if len(rows) == 0 {
		sb.WriteString(`<p class="text-sm text-muted">Nothing yet.</p></div>`)
		return
	}

	peak := rows[0].Count
	sb.WriteString(`<table class="traffic-table">`)
	for _, row := range rows {
		pct := 0
		if peak > 0 {
			pct = row.Count * 100 / peak
		}
		// The bar behind the row is the comparison; the number is the fact.
		fmt.Fprintf(sb, `<tr><td class="traffic-key"><span class="traffic-rowbar" style="width:%d%%"></span><span>%s</span></td><td class="traffic-n">%d</td></tr>`,
			pct, html.EscapeString(row.Key), row.Count)
	}
	sb.WriteString(`</table></div>`)
}

const trafficCSS = `<style>
.traffic-stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(110px,1fr));gap:12px}
.traffic-stat{display:flex;flex-direction:column;gap:2px}
.traffic-stat-n{font-size:24px;font-weight:600;font-variant-numeric:tabular-nums}
.traffic-stat-l{font-size:12px;color:var(--text-muted)}
.traffic-tabs{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:10px}
.traffic-tab{font-size:13px;color:var(--text-muted);text-decoration:none}
.traffic-tab.active{color:var(--text-primary);font-weight:600}
.traffic-chart svg{width:100%;height:160px;display:block}
.traffic-bar{fill:var(--accent-color,#000);opacity:.75}
.traffic-bar:hover{opacity:1}
.traffic-axis{display:flex;justify-content:space-between;font-size:11px;color:var(--text-muted);margin-top:4px}
.traffic-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:0 16px}
.traffic-table{width:100%;border-collapse:collapse;font-size:13px}
.traffic-table td{padding:5px 0;border:0}
.traffic-key{position:relative;padding-left:6px!important;word-break:break-all}
.traffic-key span{position:relative}
.traffic-rowbar{position:absolute!important;left:0;top:0;bottom:0;background:var(--hover-background);border-radius:3px;z-index:0}
.traffic-n{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap;padding-left:10px!important}
@media only screen and (max-width:600px){
  .traffic-stat-n{font-size:20px}
}
</style>`
