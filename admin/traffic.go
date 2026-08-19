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
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

// TrafficHandler shows what this instance is being asked to do.
//
// The chart, tabs, stats and tables come from internal/usage — the caller's own
// /usage page draws the same picture from the same helpers, so the operator's
// view and a user's view can be read against each other.
func TrafficHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	win := usage.WindowFor(r.URL.Query().Get("window"))

	var sb strings.Builder
	sb.WriteString(usage.CSS)

	// Headline numbers, so the first thing on the page is the answer to "how
	// busy is it".
	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	usage.Stat(&sb, "Last hour", usage.TotalOver(usage.Minute, 60))
	usage.Stat(&sb, "Last 24 hours", usage.TotalOver(usage.Hour, 24))
	usage.Stat(&sb, "Last 7 days", usage.TotalOver(usage.Hour, 24*7))
	usage.Stat(&sb, "Last 90 days", usage.TotalOver(usage.Day, 90))
	sb.WriteString(`</div></div>`)

	// Chart.
	sb.WriteString(`<div class="card">`)
	sb.WriteString(usage.Tabs("/admin/traffic", win))
	sb.WriteString(usage.ChartSVG(usage.Series(win.Res, win.Points), win))
	sb.WriteString(`</div>`)

	// Breakdowns over the same window.
	sb.WriteString(`<div class="traffic-grid">`)
	usage.Table(&sb, "Endpoints and tools", usage.Top(win.Res, win.Points, usage.ByName, 20))
	usage.Table(&sb, "Callers", usage.Top(win.Res, win.Points, usage.ByUser, 20))
	usage.Table(&sb, "Surface", usage.Top(win.Res, win.Points, usage.BySurface, 10))
	sb.WriteString(`</div>`)

	sb.WriteString(`<p class="text-sm text-muted">Counts only — no request is stored. ` +
		`Minutes are kept for 2 hours, hours for 7 days, days for 90.</p>`)

	app.Respond(w, r, app.Response{Title: "Traffic", Description: "What this instance is being asked to do", HTML: sb.String()})
}
