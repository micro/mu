package hazards

// /hazards — what is going wrong in the physical world, now.
//
// # Why this is drawn by hand again
//
// There was a hand-drawn page here and it was deleted in favour of
// /services/hazards, the reference every service gets. The reasoning was that
// the old page was a magnitude picker, a period picker, a table and its own
// stylesheet — a form over one tool with one argument — and the derived page is
// the card plus every method with its arguments, which is strictly more.
//
// That was true and it measured the wrong thing. "Strictly more" was more than
// a bad hand-drawn page, not more than a good one. What the reference actually
// gives somebody asking whether anything is happening is four earthquakes and
// then two thousand pixels of argument tables, curl lines and POST forms —
// `within_km`, `Lowest level to include: green, orange or red`. That is a
// manual, and a manual is the right answer to "how do I call this" and the
// wrong answer to "is anything happening".
//
// So both exist and they answer different questions. This is the thing; the
// reference is how to call the thing, linked at the foot. See #1483.
//
// # Three feeds, one question
//
// Quakes from the USGS, disaster alerts from GDACS, flood warnings from the
// Environment Agency. They are three tools because they are three sources with
// three shapes, and one page because "is anything happening" does not care
// which feed knows.
//
// Fetched in parallel. Three sequential calls to three third parties is three
// timeouts stacked on a page nobody is waiting on the details of, and a feed
// that is down should cost its own section rather than the page.
//
// # Severity is a colour, not a word
//
// A list where every row looks the same is a list somebody has to read all of.
// A red alert and a green one are different at a glance here, and a magnitude
// carries a weight, because the whole purpose of the page is that you can tell
// in one look whether to keep reading.

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
)

// What the page asks for, which is deliberately not what the tools default to.
//
// The card on Home shows M4.5 and above over a day, because it has four lines
// and wants only the notable. A page has room, so it goes down to M4.0 and
// shows more of them — the same reason /news shows more than the news card.
const (
	pageMinMagnitude = 4.0
	pagePeriod       = "day"
	pageQuakes       = 12
	pageAlerts       = 8
	pageFloods       = 8
)

// Handler serves /hazards.
func Handler(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		serveJSON(w)
		return
	}

	quakes, qat, qfailed := quakeCache.read()
	alertsFound, aat, afailed := alertCache.read()
	floods, fat, ffailed := floodCache.read()

	var b strings.Builder
	b.WriteString(`<div class="hz">`)
	b.WriteString(`<p class="hz-lede">Earthquakes, disaster alerts and flood ` +
		`warnings, from the USGS, GDACS and the Environment Agency. ` +
		`Refreshed in the background every five minutes.</p>`)

	b.WriteString(quiet("Quake feed: " + freshness(qat, qfailed)))
	if !qat.IsZero() {
		b.WriteString(quakeSection(quakes))
	}
	b.WriteString(quiet("Alert feed: " + freshness(aat, afailed)))
	if !aat.IsZero() {
		b.WriteString(alertSection(alertsFound))
	}
	b.WriteString(quiet("Flood feed: " + freshness(fat, ffailed)))
	if !fat.IsZero() {
		b.WriteString(floodSection(floods))
	}

	// How to call it, at the foot, for the reader who has just seen the answer
	// and wants it in their own program. That is the moment the reference is
	// wanted, which is why it is here and not instead of all this.
	b.WriteString(`<p class="hz-ref"><a href="/services/hazards">` +
		`Call this from your own code</a></p>`)
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{
		Title:       "Hazards",
		Description: "Earthquakes, disaster alerts and flood warnings, live",
		HTML:        b.String(),
	})
}

// gather reads public snapshots without waiting for network requests.
func gather() ([]quake, []alert, []flood) {
	qs, _, _ := quakeCache.read()
	als, _, _ := alertCache.read()
	fls, _, _ := floodCache.read()
	return qs, als, fls
}

func quakeSection(qs []quake) string {
	var b strings.Builder
	b.WriteString(section("Earthquakes",
		fmt.Sprintf("M%.1f and above, past 24 hours", pageMinMagnitude)))

	if len(qs) == 0 {
		return b.String() + quiet("Nothing above M"+
			fmt.Sprintf("%.1f", pageMinMagnitude)+" in the past day.")
	}

	b.WriteString(`<ul class="hz-list">`)
	for i, q := range qs {
		if i >= pageQuakes {
			break
		}
		extra := ""
		if q.Tsunami {
			extra = `<span class="hz-tag hz-tsunami">tsunami</span>`
		}
		b.WriteString(`<li class="hz-row">` +
			`<span class="hz-mag ` + magClass(q.Magnitude) + `">M` +
			fmt.Sprintf("%.1f", q.Magnitude) + `</span>` +
			`<span class="hz-what">` + html.EscapeString(q.Place) + extra + `</span>` +
			`<span class="hz-when">` + html.EscapeString(when(q.When)) + `</span></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func alertSection(as []alert) string {
	var b strings.Builder
	b.WriteString(section("Disaster alerts", "Orange and red, worldwide, from GDACS"))

	if len(as) == 0 {
		return b.String() + quiet("No alerts above green anywhere.")
	}

	b.WriteString(`<ul class="hz-list">`)
	for i, a := range as {
		if i >= pageAlerts {
			break
		}
		// A named event and a country, or whichever of the two there is. GDACS
		// leaves Name empty for a lot of floods, and joining unconditionally
		// rendered those as "— Nepal": a dash with nothing on the left of it.
		where := describe(a.Name, a.Country)
		b.WriteString(`<li class="hz-row">` +
			`<span class="hz-level ` + levelClass(a.Level) + `">` +
			html.EscapeString(a.Kind) + `</span>` +
			`<span class="hz-what">` + html.EscapeString(where) + `</span>` +
			`<span class="hz-when">` + html.EscapeString(when(a.From)) + `</span></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func floodSection(fs []flood) string {
	var b strings.Builder
	// Named as what it is rather than as "Floods". This feed is England's, and
	// a section headed Floods on a page whose other two are worldwide would
	// read as the world having no floods outside England.
	b.WriteString(section("Flood warnings", "In force in England, from the Environment Agency"))

	if len(fs) == 0 {
		return b.String() + quiet("No flood warnings in force in England.")
	}

	b.WriteString(`<ul class="hz-list">`)
	for i, f := range fs {
		if i >= pageFloods {
			break
		}
		b.WriteString(`<li class="hz-row">` +
			`<span class="hz-level hz-warn">flood</span>` +
			`<span class="hz-what">` + html.EscapeString(f.Area) + `</span>` +
			`<span class="hz-when">` + html.EscapeString(when(f.Raised)) + `</span></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// describe joins whichever of the two parts exist.
func describe(name, country string) string {
	name, country = strings.TrimSpace(name), strings.TrimSpace(country)
	switch {
	case name != "" && country != "":
		return name + " — " + country
	case name != "":
		return name
	}
	return country
}

// when is a time somebody can read, or nothing at all.
//
// The Environment Agency does not date every warning, and a zero time through
// ago() renders as "106751d ago" — nearly three hundred years, printed with a
// straight face next to a river in Worcestershire. A missing time is missing;
// the honest rendering of it is a blank.
func when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return ago(t)
}

func section(title, note string) string {
	return `<div class="hz-head"><h2>` + html.EscapeString(title) + `</h2>` +
		`<span class="hz-note">` + html.EscapeString(note) + `</span></div>`
}

// quiet is a section with nothing in it, which is the good news and should read
// like it rather than like a failure.
func quiet(text string) string {
	return `<p class="hz-quiet">` + html.EscapeString(text) + `</p>`
}

// magClass weights a magnitude, so the page can be read without reading it.
func magClass(m float64) string {
	switch {
	case m >= 6.5:
		return "hz-sev"
	case m >= 5.5:
		return "hz-warn"
	}
	return "hz-ok"
}

func levelClass(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "red":
		return "hz-sev"
	case "orange":
		return "hz-warn"
	}
	return "hz-ok"
}

// serveJSON is the same three answers for something that is not a browser.
func serveJSON(w http.ResponseWriter) {
	qs, as, fs := gather()
	app.RespondJSON(w, map[string]interface{}{
		"quakes": qs,
		"alerts": as,
		"floods": fs,
		"at":     time.Now().UTC(),
		"feeds":  map[string]interface{}{"quakes": feedInfo(&quakeCache), "alerts": feedInfo(&alertCache), "floods": feedInfo(&floodCache)},
	})
}
