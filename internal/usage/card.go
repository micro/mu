package usage

// What this account has been doing, on the page about the account.
//
// Usage was a sidebar entry, and before that it was in the top group beside
// Inbox, Agents, Tools and Services — where it closed the product's own three
// levels as though it were a fourth thing you do. It is not a thing you do. It
// is what this account has spent, and the reasoning that moved it down to sit
// with the account did not go far enough: the balance it spends from is a card
// on /account, so the picture of it belongs there too, not in a rail beside it.
//
// So this is the card, and /usage is still the page. The card answers "is
// anything happening, and roughly how much" at a glance; the page answers "on
// what, and what did it cost", which needs the ledger and a window switcher and
// is worth a click. A card that tried to be the page would be the page.
//
// # Why it lives here
//
// /account is served by account/ and /usage by home/, and home/ already imports
// account/ for the ledger. A card in either would be a sideways product import
// and, in one direction, a cycle. It reads nothing but the counters, which are
// here — so here is where it goes, and neither product package learns about the
// other.

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
var CardWindow = Windows[1]

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
		return ""
	}

	var sb strings.Builder
	sb.WriteString(CSS + cardCSS)
	sb.WriteString(`<div class="card usage-card">`)
	sb.WriteString(`<span class="card-title">Usage</span>`)
	sb.WriteString(`<p class="card-meta usage-card-total">` + HumanCount(total) +
		` calls in the last 7 days</p>`)
	sb.WriteString(ChartSVG(series, CardWindow))
	sb.WriteString(`<p class="card-meta"><a href="/usage?window=` + CardWindow.Slug +
		`">What you used, and what it cost →</a></p>`)
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
