package flights

// The scope: a plan view of what is around a point.
//
// The page began as a table, which is the wrong shape for the question. "What is
// that overhead" is spatial — the answer is a direction and a distance, and a
// column of numbers makes a reader do the geometry in their head. A radar is
// what the question already looks like.
//
// It draws with no map underneath and no library on top. A tile layer would mean
// a script from somebody else's CDN and an image request per pane per pan, which
// is a third party added to a page that currently has none and an instance that
// stops working when it is run somewhere without the open internet. A scope needs
// none of that: the provider already returns distance and bearing from the point
// asked about, which is polar coordinates, which is a radar display. Everything
// here is server-rendered SVG with no request of its own.
//
// North is up and the rings are nautical miles, because that is what the numbers
// beside them are. Aircraft point the way they are travelling. Nothing moves —
// this is a photograph of the sky, not a screen to watch.

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
)

// scopeSize is the SVG's internal coordinate space. The page scales it; this is
// only the unit everything below is expressed in.
const scopeSize = 600

// maxLabels caps the callsigns drawn. Heathrow returns sixty aircraft and most
// of them are parked within a mile of each other, so labelling all of them
// produces a grey smear where the airport should be.
const maxLabels = 14

// The space a callsign takes, in the SVG's own units, used to decide whether two
// labels would sit on top of each other. Roughly seven characters at ten pixels,
// drawn eight to the right of its mark.
const (
	labelWidth  = 58
	labelHeight = 13
)

// scope renders the aircraft around a point as an SVG plan view.
func scope(label string, radiusNM int, all []Aircraft) string {
	if len(all) == 0 {
		return ""
	}
	const c = scopeSize / 2
	// A margin so a mark and its label at the very edge are not clipped.
	const r = c - 34

	nm := func(d float64) float64 { return d / float64(radiusNM) * r }

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="fl-scope"><svg viewBox="0 0 %d %d" role="img" aria-label="Aircraft within %d nautical miles of %s">`,
		scopeSize, scopeSize, radiusNM, html.EscapeString(label))

	// Range rings, at a third, two thirds and the full radius, each labelled
	// with the distance it stands for.
	for _, f := range []float64{1.0 / 3, 2.0 / 3, 1} {
		rr := r * f
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%.1f" class="fl-ring"/>`, c, c, rr)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" class="fl-ringlabel">%.0f</text>`,
			c+4, float64(c)-rr+12, float64(radiusNM)*f)
	}
	// Cardinal spokes and their letters.
	fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" class="fl-spoke"/>`, c, float64(c)-r, c, float64(c)+r)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" class="fl-spoke"/>`, float64(c)-r, c, float64(c)+r, c)
	for _, p := range []struct {
		t    string
		x, y float64
	}{
		{"N", float64(c), float64(c) - r - 8},
		{"S", float64(c), float64(c) + r + 18},
		{"E", float64(c) + r + 12, float64(c) + 4},
		{"W", float64(c) - r - 12, float64(c) + 4},
	} {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="fl-cardinal">%s</text>`, p.x, p.y, p.t)
	}
	// The point everything is measured from.
	fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="3" class="fl-centre"/>`, c, c)

	// Place everything before drawing anything, because both the labelling and
	// the drawing order depend on where the marks landed.
	type mark struct {
		a     Aircraft
		x, y  float64
		id    string
		label bool
	}
	marks := make([]*mark, 0, len(all))
	for _, a := range all {
		if a.Distance > float64(radiusNM) {
			continue
		}
		// Bearing is degrees clockwise from north; the SVG's y grows downward.
		rad := a.Bearing * math.Pi / 180
		d := nm(a.Distance)
		marks = append(marks, &mark{
			a:  a,
			x:  float64(c) + d*math.Sin(rad),
			y:  float64(c) - d*math.Cos(rad),
			id: markID(a),
		})
	}

	// Label the nearest, but only where there is room.
	//
	// Nearest-first alone put seven callsigns inside the innermost ring, on top
	// of one another, which is less legible than no labels at all: the marks
	// they were meant to name disappeared underneath them. So a label is taken
	// only if nothing already placed is close enough to collide with it.
	sort.Slice(marks, func(i, j int) bool { return marks[i].a.Distance < marks[j].a.Distance })
	placed := 0
	var taken []*mark
	for _, m := range marks {
		if placed >= maxLabels || m.id == "" || m.a.OnGround {
			continue
		}
		clear := true
		for _, t := range taken {
			if math.Abs(m.x-t.x) < labelWidth && math.Abs(m.y-t.y) < labelHeight {
				clear = false
				break
			}
		}
		if clear {
			m.label = true
			taken = append(taken, m)
			placed++
		}
	}

	// Furthest first, so the nearest aircraft are drawn last and sit on top of
	// anything they overlap.
	sort.Slice(marks, func(i, j int) bool { return marks[i].a.Distance > marks[j].a.Distance })

	for _, m := range marks {
		if m.id != "" {
			fmt.Fprintf(&b, `<a href="/flights?q=%s">`, html.EscapeString(m.id))
		}
		fmt.Fprintf(&b, `<title>%s</title>`, html.EscapeString(m.a.Describe(true)))

		if m.a.OnGround {
			// A square, not an arrow: something stationary has no direction
			// worth drawing, and a heading on a parked aeroplane is noise.
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="5" height="5" class="fl-ground"/>`, m.x-2.5, m.y-2.5)
		} else {
			fmt.Fprintf(&b, `<path d="M0,-7 L4.5,6 L0,3 L-4.5,6 Z" transform="translate(%.1f,%.1f) rotate(%.1f)" class="%s"/>`,
				m.x, m.y, m.a.Track, altitudeClass(m.a.Altitude))
		}
		if m.label {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="fl-tag">%s</text>`, m.x+8, m.y+3, html.EscapeString(m.id))
		}
		if m.id != "" {
			b.WriteString(`</a>`)
		}
	}

	b.WriteString(`</svg>`)
	fmt.Fprintf(&b, `<p class="fl-legend">North up, rings in nautical miles from %s. `+
		`Arrows point the way each aircraft is travelling; squares are on the ground. `+
		`Darker aircraft are lower — the pale ones are crossing at cruise.</p></div>`, html.EscapeString(label))
	return b.String()
}

// markID is what an aircraft is called on the scope and what clicking it looks
// up. The callsign, then the registration — the hex address identifies an
// airframe perfectly and means nothing to a reader.
func markID(a Aircraft) string {
	if a.Callsign != "" {
		return a.Callsign
	}
	return a.Reg
}

// altitudeClass shades an aircraft by how high it is, so a glance separates
// something on approach from something crossing at cruise. Three bands rather
// than a gradient: the question is which layer it is in, not its exact height,
// and that is what the label beside it is for.
func altitudeClass(feet int) string {
	switch {
	case feet < 8000:
		return "fl-low"
	case feet < 24000:
		return "fl-mid"
	default:
		return "fl-high"
	}
}

const scopeCSS = `
.fl-scope{margin:12px 0}
.fl-scope svg{width:100%;max-width:560px;height:auto;display:block;margin:0 auto}
.fl-ring{fill:none;stroke:#ececec;stroke-width:1}
.fl-spoke{stroke:#f2f2f2;stroke-width:1}
.fl-centre{fill:#c00}
.fl-cardinal{font-size:13px;fill:#bbb;text-anchor:middle;font-family:inherit}
.fl-ringlabel{font-size:10px;fill:#ccc;font-family:inherit}
.fl-tag{font-size:10px;fill:#888;font-family:inherit}
.fl-ground{fill:#d8d8d8}
.fl-low{fill:#1a1a1a}
.fl-mid{fill:#777}
.fl-high{fill:#b8b8b8}
.fl-scope a{cursor:pointer}
.fl-scope a:hover .fl-low,.fl-scope a:hover .fl-mid,.fl-scope a:hover .fl-high{fill:#007bff}
.fl-legend{font-size:12px;color:#888;text-align:center;margin:8px 0 0}
`
