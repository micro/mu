package weather

// The page you get when you go looking for the weather.
//
// There was not one. /weather answered JSON to a caller that asked for JSON and
//404ed everybody else, on the reasoning that this service draws no page of its
// own and /services/weather — the generic rendering, with the card and a form
// per method — was where a person should go. Reported by somebody on their own
// instance: there is no /weather, so I cannot actually get a forecast for any
// location.
//
// The reasoning was right about one thing and wrong about the rest. A service
// that has nothing particular to show is better served by the derived page than
// by a bespoke one nobody maintains. Weather is not that service. It is the one
// where the answer is a shape — now, and the days after — and where the second
// question everybody has is "what about somewhere else", which a form built out
// of a method signature asks for as a pair of decimal coordinates.
//
// So: your place on arrival, a box for anywhere else, and the five days. Every
// other service with something to show does this — /news, /places, /video — and
// weather was the odd one out pointing at /services/weather in its Spec.
//
// # Geocoding without a key, and without importing places
//
// Searching needs a name turned into coordinates. service/places does that and
// this package may not import it — services do not call each other, see
// AGENTS.md — and the answer is not to add a hook for one form. Open-Meteo,
// which is already the keyless forecast provider here, publishes a geocoder on
// the same terms. One provider, no key, no new dependency in either direction.

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// Place is one geocoding result.
type Place struct {
	Name    string  `json:"name"`
	Country string  `json:"country"`
	Admin1  string  `json:"admin1"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
}

// Label is the place as somebody would say it: the town, then the region and
// country when they add something. "Cambridge" alone is three places.
func (p Place) Label() string {
	parts := []string{strings.TrimSpace(p.Name)}
	for _, extra := range []string{p.Admin1, p.Country} {
		if e := strings.TrimSpace(extra); e != "" && e != parts[len(parts)-1] {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, ", ")
}

// geocodeURL is where a name is turned into coordinates. A variable so a test
// can point it at a server it controls rather than at the internet.
var geocodeURL = "https://geocoding-api.open-meteo.com/v1/search"

// geocode resolves a place name. It returns at most a handful: the first is
// used and the rest are offered, because "Cambridge" is a real ambiguity and
// silently picking one is how somebody ends up reading the wrong forecast.
func geocode(ctx context.Context, name string) ([]Place, error) {
	q := strings.TrimSpace(name)
	if q == "" {
		return nil, nil
	}
	u := geocodeURL + "?count=5&language=en&format=json&name=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	rsp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the place lookup answered %d", rsp.StatusCode)
	}
	var out struct {
		Results []Place `json:"results"`
	}
	if err := json.NewDecoder(rsp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// PageHandler serves GET /weather as a page.
//
// The JSON caller is served first and unchanged — see Handler, which routes on
// what the request asked for. Everything below is the HTML half.
func PageHandler(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	accountID := ""
	if acc != nil {
		accountID = acc.ID
	}

	// POST: the place somebody looks up is where they are, or where they are
	// going. Either is a fact about them and not one to leave in a URL — see
	// AGENTS.md, "What may travel in a URL".
	q := strings.TrimSpace(r.PostFormValue("q"))
	var b strings.Builder
	b.WriteString(`<div class="wx-page">`)
	b.WriteString(searchForm(q, auth.CSRFToken(r)))

	switch {
	case q != "":
		b.WriteString(forPlace(r.Context(), q))
	default:
		b.WriteString(forYou(r.Context(), accountID))
	}

	b.WriteString(`</div>` + pageCSS)
	app.Respond(w, r, app.Response{
		Title:       "Weather",
		Description: "The forecast where you are, and anywhere else",
		HTML:        b.String(),
	})
}

func searchForm(q, csrf string) string {
	return `<form class="wx-find" method="POST" action="/weather">` + app.CSRFField(csrf) + `
  <input class="field" type="search" name="q" placeholder="Anywhere — a town, a city" ` +
		`value="` + html.EscapeString(q) + `" maxlength="120">
  <button class="btn" type="submit">Look up</button>
</form>`
}

// forYou is the forecast where the account says it is.
//
// Three outcomes and each says which it is. The one that used to be missing is
// the middle: an account with no location got nothing at all, so the page
// looked broken rather than unconfigured.
func forYou(ctx context.Context, accountID string) string {
	if accountID == "" {
		return `<p class="wx-note">Sign in and set where you are to get your own ` +
			`forecast, or look up any place above.</p>`
	}
	lat, lon, located := auth.Located(accountID)
	if !located {
		return `<p class="wx-note">This account has no location yet, so there is no ` +
			`"here" to forecast. ` + app.TextLink("Set where you are", "/account/place") +
			`, or look up any place above.</p>`
	}
	f, err := FetchWeather(ctx, lat, lon)
	if err != nil || f == nil || f.Current == nil {
		where := strings.TrimSpace(auth.PlaceName(accountID))
		if where == "" {
			where = "your location"
		}
		return `<p class="wx-note">No forecast for ` + html.EscapeString(where) +
			` right now — the weather provider did not answer. It is set in ` +
			app.TextLink("configuration", "/admin/config") + `.</p>`
	}
	return forecastHTML(f, airFor(lat, lon), auth.PlaceName(accountID))
}

// forPlace is the forecast somewhere else, with the other matches offered.
func forPlace(ctx context.Context, q string) string {
	found, err := geocode(ctx, q)
	if err != nil {
		return `<p class="wx-note">Could not look that up: ` +
			html.EscapeString(err.Error()) + `.</p>`
	}
	if len(found) == 0 {
		return `<p class="wx-note">Nowhere called ` + html.EscapeString(q) +
			` — try a town or a city.</p>`
	}

	first := found[0]
	f, err := FetchWeather(ctx, first.Lat, first.Lon)
	var b strings.Builder
	if err != nil || f == nil || f.Current == nil {
		b.WriteString(`<p class="wx-note">Found ` + html.EscapeString(first.Label()) +
			`, and the forecast did not come back.</p>`)
	} else {
		b.WriteString(forecastHTML(f, airFor(first.Lat, first.Lon), first.Label()))
	}

	// The other matches. "Cambridge" is a real ambiguity and picking one
	// silently is how somebody reads the wrong forecast and believes it.
	if len(found) > 1 {
		b.WriteString(`<div class="wx-also"><span class="wx-also-head">Also called ` +
			html.EscapeString(strings.TrimSpace(q)) + `</span>`)
		for _, p := range found[1:] {
			b.WriteString(` <a class="pill" href="/weather?q=` +
				html.EscapeString(url.QueryEscape(p.Label())) + `">` +
				html.EscapeString(p.Label()) + `</a>`)
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

// forecastHTML is the forecast as a page, rather than as a card.
//
// The card is three facts — a number, a word and five days — because it sits in
// a column on Home next to eight other cards and has one line to earn. A page
// somebody navigated to on purpose is a different question, and answering it
// with the card was the whole complaint: any weather site tells you what the
// day is like, and this told you a temperature.
//
// None of this costs a second request. FeelsLikeC, Humidity, WindKph, Sunrise,
// Sunset, RainMM and the hourly series were all in the response already, parsed
// into the struct, and thrown away by every renderer there was — the fetch has
// always paid for them.
func forecastHTML(f *WeatherForecast, air *AirQuality, place string) string {
	where := strings.TrimSpace(place)
	if where == "" {
		where = f.Location
	}
	c := f.Current

	var b strings.Builder
	b.WriteString(`<div class="wx-full">`)
	if where != "" {
		b.WriteString(`<h2 class="wx-place">` + html.EscapeString(where) + `</h2>`)
	}

	// Now: the number, then the sentence somebody would say.
	b.WriteString(`<div class="wx-now-row">` +
		`<span class="wx-big">` + deg(c.TempC) + `</span>` +
		`<span class="wx-said">` + html.EscapeString(c.Description) + `</span></div>`)

	// The facts under it, and only the ones this provider actually returned.
	// A dash where a number should be is worse than a shorter list.
	var facts []string
	if math.Abs(c.FeelsLikeC-c.TempC) >= 1 {
		facts = append(facts, fact("Feels like", deg(c.FeelsLikeC)))
	}
	if c.WindKphAvailable {
		facts = append(facts, fact("Wind", strconv.Itoa(int(math.Round(c.WindKph)))+" km/h"))
	}
	if c.HumidityAvailable {
		facts = append(facts, fact("Humidity", strconv.Itoa(c.Humidity)+"%"))
	}
	if len(f.DailyItems) > 0 {
		d := f.DailyItems[0]
		facts = append(facts, fact("Today", deg(d.MaxTempC)+" / "+deg(d.MinTempC)))
		// A percentage where the provider gives one, millimetres where it does
		// not, and nothing at all where neither says anything. It read "Rain
		// 0.0 mm" next to a day described as "Light rain", which is two
		// statements about the same day that cannot both be true.
		switch {
		case d.RainChance > 0:
			facts = append(facts, fact("Rain", strconv.Itoa(d.RainChance)+"%"))
		case d.RainMM > 0:
			facts = append(facts, fact("Rain", oneDP(d.RainMM)+" mm"))
		}
		if !d.Sunrise.IsZero() && !d.Sunset.IsZero() {
			facts = append(facts, fact("Light",
				d.Sunrise.Format("15:04")+" – "+d.Sunset.Format("15:04")))
		}
	}
	// UV and air, which are keyless and were already being fetched for the JSON
	// caller while no page showed them. UV decides an afternoon outdoors more
	// often than the temperature does.
	if air != nil {
		if air.UV > 0 {
			facts = append(facts, fact("UV", oneDP(air.UV)))
		}
		if air.HaveEuropeanAQI && air.EuropeanAQI > 0 {
			facts = append(facts, fact("Air", strconv.Itoa(air.EuropeanAQI)+" "+aqiWord(air.EuropeanAQI)))
		}
	}
	if len(facts) > 0 {
		b.WriteString(`<div class="wx-facts">` + strings.Join(facts, "") + `</div>`)
	}

	// The next few hours, which is the question "do I need a coat this
	// afternoon" and the one a daily high cannot answer.
	if hours := nextHours(f.HourlyItems, 8); len(hours) > 0 {
		b.WriteString(`<div class="wx-strip"><span class="wx-strip-head">Next hours</span>` +
			`<div class="wx-strip-row">`)
		for _, h := range hours {
			b.WriteString(`<span class="wx-hour"><span class="wx-hour-at">` +
				html.EscapeString(h.Time.Format("15:04")) + `</span>` +
				`<span class="wx-hour-t">` + deg(h.TempC) + `</span></span>`)
		}
		b.WriteString(`</div></div>`)
	}

	// And the days, with what each one is rather than only its numbers.
	if len(f.DailyItems) > 0 {
		b.WriteString(`<div class="wx-strip"><span class="wx-strip-head">The days after</span>` +
			`<div class="wx-list">`)
		for i, d := range f.DailyItems {
			if i >= 6 {
				break
			}
			when := d.Date.Format("Mon 2 Jan")
			if i == 0 {
				when = "Today"
			}
			b.WriteString(`<div class="wx-row"><span class="wx-row-day">` +
				html.EscapeString(when) + `</span>` +
				`<span class="wx-row-said">` + html.EscapeString(d.Description) + `</span>` +
				`<span class="wx-row-t">` + deg(d.MaxTempC) + ` / ` + deg(d.MinTempC) + `</span></div>`)
		}
		b.WriteString(`</div></div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

// nextHours is the hours still to come, capped. A series that starts this
// morning is half in the past by the afternoon, and showing 09:00 at 4pm is the
// kind of wrong that makes somebody distrust the rest of the page.
func nextHours(all []HourlyItem, n int) []HourlyItem {
	now := time.Now()
	var out []HourlyItem
	for _, h := range all {
		if h.Time.Before(now) {
			continue
		}
		out = append(out, h)
		if len(out) >= n {
			break
		}
	}
	return out
}

func fact(name, value string) string {
	return `<span class="wx-fact"><span class="wx-fact-n">` + html.EscapeString(name) +
		`</span><span class="wx-fact-v">` + html.EscapeString(value) + `</span></span>`
}

func deg(c float64) string { return strconv.Itoa(int(math.Round(c))) + "°" }

func oneDP(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// airFor is the air quality, or nil. Never an error: this is one line on a
// page about something else, and a provider that did not answer should cost the
// forecast nothing.
func airFor(lat, lon float64) *AirQuality {
	air, err := airQuality(lat, lon)
	if err != nil {
		return nil
	}
	return air
}

const pageCSS = `<style>
.wx-page{max-width:760px}
.wx-find{display:flex;gap:8px;margin:0 0 var(--spacing-lg,24px);flex-wrap:wrap}
.wx-find .field{flex:1;min-width:0}
.wx-note{color:var(--text-secondary,#555);line-height:1.6;margin:0}
.wx-also{margin:var(--spacing-lg,24px) 0 0;display:flex;align-items:baseline;gap:6px;flex-wrap:wrap}
.wx-also-head{font-size:12px;color:var(--text-muted,#999);text-transform:uppercase;letter-spacing:.04em}
.wx-place{margin:0 0 4px;font-size:18px}
.wx-now-row{display:flex;align-items:baseline;gap:12px;flex-wrap:wrap;margin:0 0 var(--spacing-md,16px)}
.wx-big{font-size:44px;line-height:1;font-weight:600;color:var(--text-primary,#111)}
.wx-said{font-size:16px;color:var(--text-secondary,#555)}
.wx-facts{display:flex;gap:22px;flex-wrap:wrap;margin:0 0 var(--spacing-lg,24px);
  padding:0 0 var(--spacing-md,16px);border-bottom:1px solid var(--card-border,#eee)}
.wx-fact{display:flex;flex-direction:column;gap:2px}
.wx-fact-n{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-muted,#999)}
.wx-fact-v{font-size:15px;color:var(--text-primary,#111);font-variant-numeric:tabular-nums}
.wx-strip{margin:0 0 var(--spacing-lg,24px)}
.wx-strip-head{display:block;font-size:11px;text-transform:uppercase;letter-spacing:.04em;
  color:var(--text-muted,#999);margin:0 0 8px}
/* The hours scroll rather than wrap: eight of them on a phone is two ragged
   rows, and a row of times reads as a row. */
.wx-strip-row{display:flex;gap:18px;overflow-x:auto;padding:0 0 4px}
.wx-hour{display:flex;flex-direction:column;gap:2px;flex:none;text-align:center}
.wx-hour-at{font-size:11px;color:var(--text-muted,#999)}
.wx-hour-t{font-size:15px;font-variant-numeric:tabular-nums}
.wx-list{display:flex;flex-direction:column}
.wx-row{display:flex;align-items:baseline;gap:12px;padding:7px 0;
  border-bottom:1px solid var(--card-border,#f0f0f0)}
.wx-row:last-child{border-bottom:0}
.wx-row-day{flex:none;min-width:104px;font-size:14px}
.wx-row-said{flex:1;min-width:0;font-size:13px;color:var(--text-secondary,#555);
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.wx-row-t{flex:none;font-size:14px;font-variant-numeric:tabular-nums;color:var(--text-secondary,#555)}
@media (max-width:600px){.wx-row-said{display:none}}
</style>`
