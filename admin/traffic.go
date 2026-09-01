package admin

// The operator's answer to "what is this instance doing?".
//
// Two directions of one question, and they were two nav entries. Requests is
// what people and agents ask of it, over time and broken down by endpoint,
// tool, caller and surface. Spend is what that costs us in third-party APIs.
// An operator seeing a spike wants both at once — the traffic that caused it
// and the bill it is running up — and had to open two pages to get them.
//
// One page, two tabs, the same shape as Logs. Requests first because it is the
// one that is looked at daily.
//
// The chart is a server-rendered SVG. A page of bars needs no library, no CDN
// (the CSP would refuse one anyway) and no client-side fetching, and it is the
// same data whether the operator is on a laptop or a phone.

import (
	"html"
	"net/http"
	"net/url"
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

	spend := r.URL.Query().Get("tab") == "spend"

	var sb strings.Builder
	sb.WriteString(usage.CSS)
	sb.WriteString(trafficTabs(spend))
	if spend {
		sb.WriteString(spendCard())
		app.Respond(w, r, app.Response{Title: "Spend",
			Description: "What this instance spends on third parties", HTML: sb.String()})
		return
	}

	win := usage.WindowFor(r.URL.Query().Get("window"))

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
	// The window switcher keeps the caller, if there is one — see TabsWith.
	var keep url.Values
	if c := strings.TrimSpace(r.URL.Query().Get("caller")); c != "" {
		keep = url.Values{"caller": {c}}
	}
	sb.WriteString(usage.TabsWith("/admin/traffic", win, keep))
	sb.WriteString(usage.ChartSVG(usage.Series(win.Res, win.Points), win))
	sb.WriteString(`</div>`)

	// One caller, if the Callers table was clicked through.
	//
	// The whole table listed who was busy and nothing said what any of them
	// were doing — the two facts were on the same page, one row apart, and
	// unconnectable. "asim: 400" is a number an operator cannot act on.
	who := strings.TrimSpace(r.URL.Query().Get("caller"))

	// Breakdowns over the same window.
	sb.WriteString(`<div class="traffic-grid">`)
	usage.Table(&sb, "Endpoints and tools", usage.Top(win.Res, win.Points, usage.ByName, 20))

	// Each caller links to their own breakdown, in the window being looked at
	// — a drill-down that silently changed the window would answer a different
	// question from the one the row was showing.
	usage.LinkTable(&sb, "Callers", usage.Top(win.Res, win.Points, usage.ByUser, 20), who,
		func(key string) string {
			if key == usage.Other {
				return "" // the tail past the cap, not a caller
			}
			q := url.Values{"caller": {key}}
			if win.Slug != "" {
				q.Set("window", win.Slug)
			}
			return "/admin/traffic?" + q.Encode()
		})

	usage.Table(&sb, "Surface", usage.Top(win.Res, win.Points, usage.BySurface, 10))
	sb.WriteString(`</div>`)

	// The drill-down below the grid, not inside it.
	//
	// It was a fourth cell in a three-up auto-fit grid, so opening a caller
	// reflowed the other three into different columns — the page rearranged
	// itself around the thing you had just clicked, which reads as the click
	// having broken something.
	if who != "" {
		rows, rest := usage.TopFor(win.Res, win.Points, who, 20)
		sb.WriteString(`<div class="traffic-drill">`)
		usage.TableWithRest(&sb, who+" is calling", rows, rest)
		sb.WriteString(`<p class="text-sm"><a href="/admin/traffic?window=` +
			html.EscapeString(win.Slug) + `">&larr; All callers</a></p>`)
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`<p class="text-sm text-muted">Counts only — no request is stored. ` +
		`Minutes are kept for 2 hours, hours for 7 days, days for 90.</p>`)

	app.Respond(w, r, app.Response{Title: "Usage",
		Description: "What this instance is being asked to do", HTML: sb.String()})
}

// trafficTabs is the switch between what came in and what it cost.
func trafficTabs(spend bool) string {
	// app.PillLink — see the note on the Logs tabs, which had the same
	// invented class and the same invisible selected state.
	return `<div class="d-flex gap-2 mb-3">` +
		app.PillLink("Requests", "/admin/traffic", !spend) +
		app.PillLink("Spend", "/admin/traffic?tab=spend", spend) +
		`</div>`
}

// SpendMoved sends the old address to its tab.
func SpendMoved(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/traffic?tab=spend", http.StatusSeeOther)
}
