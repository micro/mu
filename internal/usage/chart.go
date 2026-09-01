package usage

// Drawing a series.
//
// Server-rendered SVG. A page of bars needs no library, no CDN (the CSP would
// refuse one anyway) and no client-side fetching, and it is the same picture
// whether it is read on a laptop or a phone.
//
// This lives here rather than in the page that first drew it because there are
// two pages now — the operator's, which is everyone's traffic, and the
// caller's, which is their own — and a chart that differs between them would
// make the two impossible to compare.

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

// Window is a span of a series: how far back, at what resolution, and how a
// bucket's time is labelled.
type Window struct {
	Slug   string
	Label  string
	Res    Resolution
	Points int
	Format string
}

// Windows are the three spans a usage page offers: what is happening now, when
// it is busy, and whether it is growing.
var Windows = []Window{
	{"live", "Last 2 hours", Minute, 120, "15:04"},
	{"week", "Last 7 days", Hour, 24 * 7, "Mon 15:00"},
	{"quarter", "Last 90 days", Day, 90, "2 Jan"},
}

// WindowFor resolves a slug to a window, defaulting to the first.
func WindowFor(slug string) Window {
	for _, w := range Windows {
		if w.Slug == slug {
			return w
		}
	}
	return Windows[0]
}

// Tabs renders the window switcher for a page at base (e.g. "/usage").
func Tabs(base string, current Window) string { return TabsWith(base, current, nil) }

// TabsWith is Tabs, keeping the rest of the page's state in the links.
//
// The window switcher wrote "?window=..." and nothing else, which is right
// while the window is all the page's state. It stopped being right the moment
// /admin/traffic could be looking at one caller: clicking "7 days" answered the
// same question about everybody instead of a wider version of the question you
// were asking, and the way back was the browser's back button.
func TabsWith(base string, current Window, extra url.Values) string {
	var sb strings.Builder
	sb.WriteString(`<div class="traffic-tabs">`)
	for _, w := range Windows {
		active := ""
		if w.Slug == current.Slug {
			active = " active"
		}
		q := url.Values{"window": {w.Slug}}
		for k, vs := range extra {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		fmt.Fprintf(&sb, `<a href="%s?%s" class="traffic-tab%s">%s</a>`,
			html.EscapeString(base), html.EscapeString(q.Encode()), active,
			html.EscapeString(w.Label))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// ChartSVG draws the series as bars, sized in percentages so it scales to
// whatever width it is given.
func ChartSVG(series []Bucket, win Window) string {
	const height = 160

	peak, total := 0, 0
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
		width, height, html.EscapeString(win.Label), peak)

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
			html.EscapeString(b.At.Local().Format(win.Format)), b.Total)
	}
	sb.WriteString(`</svg></div>`)

	// Axis: first, middle and last labels. More than that is unreadable at
	// phone width and says nothing extra.
	fmt.Fprintf(&sb, `<div class="traffic-axis"><span>%s</span><span>%s</span><span>%s</span></div>`,
		html.EscapeString(series[0].At.Local().Format(win.Format)),
		html.EscapeString(series[len(series)/2].At.Local().Format(win.Format)),
		html.EscapeString(series[len(series)-1].At.Local().Format(win.Format)))
	fmt.Fprintf(&sb, `<p class="text-sm text-muted">%d calls in this window · peak %d per %s</p>`,
		total, peak, strings.TrimSuffix(string(win.Res), "s"))
	return sb.String()
}

// Stat renders one headline number.
func Stat(sb *strings.Builder, label string, n int) {
	fmt.Fprintf(sb, `<div class="traffic-stat"><span class="traffic-stat-n">%s</span><span class="traffic-stat-l">%s</span></div>`,
		HumanCount(n), html.EscapeString(label))
}

// HumanCount keeps a big number readable in a small tile.
func HumanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// Table renders a breakdown, each row's bar showing it against the largest.
// LinkTable is Table with each row a link.
//
// href returns where a key goes, or "" for a row that leads nowhere — which is
// what Other is: an aggregate of the tail past the cap, not a caller. selected
// marks the row currently being drilled into.
func LinkTable(sb *strings.Builder, title string, rows []Count, selected string, href func(string) string) {
	table(sb, title, rows, selected, href, 0)
}

func Table(sb *strings.Builder, title string, rows []Count) {
	table(sb, title, rows, "", nil, 0)
}

// TableWithRest is Table plus a final muted row for what the breakdown could
// not account for. Drawn only when there is something to say — see TopFor.
func TableWithRest(sb *strings.Builder, title string, rows []Count, rest int) {
	table(sb, title, rows, "", nil, rest)
}

// table draws one breakdown.
//
// Rows are flex, not table cells. They were a <table>, and when the callers
// became clickable the only thing carrying the link was the label — so the hit
// area was one underlined word in a row twenty times its width, with no hover
// on the row and nothing showing which one you were looking at. A row that acts
// like a button has to look and behave like one across its whole width.
func table(sb *strings.Builder, title string, rows []Count, selected string, href func(string) string, rest int) {
	sb.WriteString(`<div class="card"><h3>` + html.EscapeString(title) + `</h3>`)
	if len(rows) == 0 && rest == 0 {
		sb.WriteString(`<p class="text-sm text-muted">Nothing yet.</p></div>`)
		return
	}

	peak := 0
	if len(rows) > 0 {
		peak = rows[0].Count
	}
	if rest > peak {
		peak = rest
	}

	sb.WriteString(`<div class="traffic-rows">`)
	for _, row := range rows {
		pct := 0
		if peak > 0 {
			pct = row.Count * 100 / peak
		}
		to := ""
		if href != nil {
			to = href(row.Key)
		}
		cls := "traffic-row"
		if selected != "" && row.Key == selected {
			cls += " selected"
		}
		// The bar behind the row is the comparison; the number is the fact.
		inner := fmt.Sprintf(`<span class="traffic-rowbar" style="width:%d%%"></span>`+
			`<span class="traffic-label">%s</span><span class="traffic-n">%d</span>`,
			pct, html.EscapeString(row.Key), row.Count)
		if to != "" {
			fmt.Fprintf(sb, `<a class="%s" href="%s">%s</a>`, cls, html.EscapeString(to), inner)
		} else {
			fmt.Fprintf(sb, `<div class="%s">%s</div>`, cls, inner)
		}
	}
	if rest > 0 {
		pct := 0
		if peak > 0 {
			pct = rest * 100 / peak
		}
		// Named for what it is rather than left out. See TopFor: this is the
		// difference between the caller's total and what the breakdown could
		// account for, and hiding it is what made the two tables disagree.
		fmt.Fprintf(sb, `<div class="traffic-row traffic-rest">`+
			`<span class="traffic-rowbar" style="width:%d%%"></span>`+
			`<span class="traffic-label">not broken down</span>`+
			`<span class="traffic-n">%d</span></div>`, pct, rest)
	}
	sb.WriteString(`</div></div>`)
}

// Since is a short "3 days ago" for a page that has room for one word.
func Since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// CSS is the styling both usage pages share.
const CSS = `<style>
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
.traffic-drill{margin-top:16px}
.traffic-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:0 16px}
.traffic-rows{display:flex;flex-direction:column}
.traffic-row{position:relative;display:flex;align-items:center;gap:10px;padding:6px 8px;border-radius:3px;font-size:13px;color:inherit;text-decoration:none}
.traffic-rowbar{position:absolute;left:0;top:0;bottom:0;background:var(--hover-background);border-radius:3px;z-index:0}
.traffic-label{position:relative;z-index:1;flex:1;min-width:0;word-break:break-all}
.traffic-n{position:relative;z-index:1;text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
a.traffic-row{cursor:pointer}
a.traffic-row:hover .traffic-rowbar,a.traffic-row:focus-visible .traffic-rowbar{background:var(--card-border)}
a.traffic-row:hover .traffic-label{text-decoration:underline}
a.traffic-row:focus-visible{outline:2px solid var(--text-primary);outline-offset:-2px}
.traffic-row.selected{font-weight:600}
.traffic-row.selected .traffic-rowbar{background:var(--card-border)}
.traffic-rest{color:var(--text-muted)}
.traffic-rest .traffic-rowbar{opacity:.45}
@media only screen and (max-width:600px){
  .traffic-stat-n{font-size:20px}
}
</style>`
