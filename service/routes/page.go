package routes

// The page: two ends, a way to travel, and the road drawn.
//
// The drawing is the reason this page exists rather than being a paragraph.
// Google returns the route's shape as an encoded polyline in the same response
// that gives the time, so the line is already paid for — and drawing it as SVG
// costs nothing more, where a map tile would be another product, another key
// and another bill. It is the flights radar's trick again: we have the
// coordinates, so draw them.
//
// It is not a map. There is no coastline, no street, no north arrow — just the
// shape of the journey with its ends marked. That is honest about what we have,
// and it answers the thing a shape can answer: does this route go the way I
// expected, or does it loop out to a motorway.
//
// No sign-in. A journey between two public places is public, and the page is a
// second door onto the same tools rather than a different service.

import (
	"fmt"
	"html"
	"math"
	"net/http"
	"strings"

	"mu/internal/app"
)

// Handler serves /routes.
func Handler(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))

	var b strings.Builder
	b.WriteString(form(from, to, mode))

	if from != "" && to != "" {
		b.WriteString(journeyCard(from, to, mode))
	} else {
		b.WriteString(`<div class="card"><p class="text-sm text-muted">` +
			`Two places, and how you are travelling. You get the time, how much of it ` +
			`is traffic, the turns, and the shape of the road — which is the part that ` +
			`tells you whether the route goes the way you expected.</p></div>`)
	}
	b.WriteString(pageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Routes", "How to get from one place to another", b.String(), r)))
}

// form is the two ends and the mode.
func form(from, to, mode string) string {
	var b strings.Builder
	b.WriteString(`<form method="GET" action="/routes" class="card rt-form">`)
	b.WriteString(`<input name="from" value="` + html.EscapeString(from) +
		`" placeholder="From — e.g. King's Cross, London" autocomplete="off" aria-label="Starting point">`)
	b.WriteString(`<input name="to" value="` + html.EscapeString(to) +
		`" placeholder="To — e.g. British Museum" autocomplete="off" aria-label="Destination">`)
	b.WriteString(`<select name="mode" aria-label="How to travel">`)
	for _, m := range []struct{ value, label string }{
		{"drive", "Drive"}, {"walk", "Walk"}, {"cycle", "Cycle"}, {"transit", "Transit"},
	} {
		sel := ""
		if m.value == mode || (mode == "" && m.value == "drive") {
			sel = " selected"
		}
		b.WriteString(`<option value="` + m.value + `"` + sel + `>` + m.label + `</option>`)
	}
	b.WriteString(`</select><button type="submit">Go</button></form>`)
	return b.String()
}

// journeyCard is the answer: the summary, the drawing, and the turns.
func journeyCard(from, to, mode string) string {
	j, msg := plan(&ETARequest{From: from, To: to, Mode: mode})
	if msg != "" {
		return `<div class="card"><p class="text-sm text-muted">` + html.EscapeString(msg) + `</p></div>`
	}
	r, err := computeRoute(j.fromLat, j.fromLon, j.toLat, j.toLon, j.mode, j.when, full)
	if err != nil {
		return `<div class="card"><p class="text-sm text-muted">` +
			html.EscapeString(err.Error()) + `.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	fmt.Fprintf(&b, `<h3 class="rt-head">%s → %s</h3>`, html.EscapeString(j.fromLabel), html.EscapeString(j.toLabel))
	fmt.Fprintf(&b, `<p class="rt-summary"><strong>%s</strong> · %s · by %s`,
		html.EscapeString(humanDuration(r.Duration)), html.EscapeString(humanDistance(r.Metres)),
		html.EscapeString(spoken(j.mode)))
	if d := r.Delay(); d > 0 {
		fmt.Fprintf(&b, ` · <span class="rt-traffic">%s of it traffic</span>`, html.EscapeString(humanDuration(d)))
	}
	b.WriteString(`</p>`)

	if r.Estimate {
		b.WriteString(`<p class="text-sm text-muted">Estimated from straight-line distance — ` +
			`live routing is unavailable on this instance, so treat it as approximate.</p>`)
	}
	if svg := draw(r.Shape); svg != "" {
		b.WriteString(svg)
	}
	if len(r.Steps) > 0 {
		b.WriteString(`<ol class="rt-steps">`)
		for _, s := range r.Steps {
			b.WriteString(`<li>` + html.EscapeString(s.Text))
			if s.Metres > 0 {
				b.WriteString(` <span class="rt-dist">` + html.EscapeString(humanDistance(s.Metres)) + `</span>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ol>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// drawing bounds, in the SVG's own units.
const (
	drawW   = 640
	drawH   = 320
	drawPad = 18
)

// draw is the route as an SVG path.
//
// Latitude and longitude are projected flat, with longitude squeezed by the
// cosine of the latitude — at 51 degrees north a degree of longitude is barely
// six tenths of a degree of latitude, and without the correction every route in
// Britain comes out stretched sideways. Over the length of a journey somebody
// might drive, flat is indistinguishable from correct.
func draw(shape []point) string {
	if len(shape) < 2 {
		return ""
	}
	minLat, maxLat := shape[0].Lat, shape[0].Lat
	minLon, maxLon := shape[0].Lon, shape[0].Lon
	for _, p := range shape {
		minLat, maxLat = min(minLat, p.Lat), max(maxLat, p.Lat)
		minLon, maxLon = min(minLon, p.Lon), max(maxLon, p.Lon)
	}
	// A journey that goes nowhere has no shape to draw.
	squeeze := cosDeg((minLat + maxLat) / 2)
	spanX, spanY := (maxLon-minLon)*squeeze, maxLat-minLat
	if spanX <= 0 && spanY <= 0 {
		return ""
	}
	// One scale for both axes, so the drawing keeps the journey's proportions
	// rather than stretching a straight road to fill the box.
	scale := min((drawW-2*drawPad)/nonZero(spanX), (drawH-2*drawPad)/nonZero(spanY))
	offX := (drawW - spanX*scale) / 2
	offY := (drawH - spanY*scale) / 2
	at := func(p point) (float64, float64) {
		x := offX + (p.Lon-minLon)*squeeze*scale
		// SVG y grows downward and latitude grows northward.
		y := drawH - offY - (p.Lat-minLat)*scale
		return x, y
	}

	var d strings.Builder
	for i, p := range shape {
		x, y := at(p)
		if i == 0 {
			fmt.Fprintf(&d, "M%.1f %.1f", x, y)
			continue
		}
		fmt.Fprintf(&d, " L%.1f %.1f", x, y)
	}
	sx, sy := at(shape[0])
	ex, ey := at(shape[len(shape)-1])

	return fmt.Sprintf(`<svg class="rt-map" viewBox="0 0 %d %d" role="img" `+
		`aria-label="The shape of the route from start to finish">`+
		`<path d="%s" fill="none" stroke="currentColor" stroke-width="2.5" `+
		`stroke-linejoin="round" stroke-linecap="round" opacity="0.85"/>`+
		`<circle cx="%.1f" cy="%.1f" r="5" fill="none" stroke="currentColor" stroke-width="2"/>`+
		`<circle cx="%.1f" cy="%.1f" r="5" fill="currentColor"/>`+
		`</svg>`, drawW, drawH, d.String(), sx, sy, ex, ey)
}

// nonZero keeps a straight north-south or east-west route from dividing by zero.
func nonZero(v float64) float64 {
	if v <= 0 {
		return 1e-9
	}
	return v
}

// cosDeg is the cosine of an angle in degrees.
func cosDeg(deg float64) float64 {
	c := math.Cos(deg * math.Pi / 180)
	if c < 0.05 {
		return 0.05 // near the poles, stop squeezing rather than collapse
	}
	return c
}

const pageCSS = `<style>
.rt-form{display:flex;gap:8px;flex-wrap:wrap;align-items:center}
.rt-form input{flex:1;min-width:180px;padding:8px 10px;border:1px solid var(--border-color,#d1d5db);
  border-radius:8px;font-size:14px;font-family:inherit}
.rt-form select{padding:8px 10px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;
  font-size:14px;font-family:inherit}
.rt-head{margin:0 0 6px;font-size:15px}
.rt-summary{margin:0 0 12px;font-size:14px;color:var(--text-muted,#666)}
.rt-traffic{color:#b45309}
.rt-map{display:block;width:100%;height:auto;margin:0 0 14px;color:#111}
.rt-steps{margin:0;padding-left:20px;font-size:14px;line-height:1.7}
.rt-steps li{margin:0 0 2px}
.rt-dist{color:var(--text-muted,#999);font-size:12px}
@media (prefers-color-scheme:dark){.rt-map{color:#eee}}
</style>`
