package usage

// Shared activity chart for account billing.

import (
	"strconv"
	"strings"
)

// CardWindow is what the card draws: a week, by the hour.
//
// Not the live two hours, which is the page's default and the right default
// there — you open /usage because something is happening now. A card is read in
// passing, and an hour of quiet on a two-hour chart looks like nothing is
// working. Seven days always has a shape.
var CardWindow = WindowFor("week")

// Card is one account's recent activity: the graph, a total, and the way to the
// page that explains it.
//
// Empty for an account that has never called anything. A chart of nothing is a
// flat line somebody has to interpret, and the honest version of "no calls yet"
// is not drawing a chart — /usage says it in words, and the sidebar entry that
// used to lead there for everybody is exactly what this replaces.
func Card(account string) string {
	if account == "" {
		return ""
	}
	series := SeriesFor(account, CardWindow.Res, CardWindow.Points)
	total := 0
	for _, b := range series {
		total += b.Total
	}
	if total == 0 {
		return `<section class="card"><h2>Usage</h2><p>No activity yet.</p><p><a href="/account/usage">View usage</a></p></section>`
	}

	var sb strings.Builder
	sb.WriteString(CSS + cardCSS)
	sb.WriteString(`<div class="card usage-card">`)
	sb.WriteString(`<span class="card-title">Usage</span>`)
	sb.WriteString(`<p class="card-meta usage-card-total">` + HumanCount(total) +
		` calls in the last 7 days</p>`)
	sb.WriteString(ChartSVG(series, CardWindow))
	sb.WriteString(`<p class="card-meta"><a href="/account/usage?window=` + CardWindow.Slug +
		`">View usage</a></p>`)
	sb.WriteString(`</div>`)
	return sb.String()
}

// cardCSS shortens the chart. ChartSVG is 160 high for a page that is mostly
// chart; in a card between the balance and the ledger that is a graph shouting
// over its neighbours.
var cardCSS = `<style>
.usage-card .traffic-chart svg{height:` + strconv.Itoa(cardChartHeight) + `px}
.usage-card-total{margin-bottom:10px}
</style>`

const cardChartHeight = 84
