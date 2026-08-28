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
	"net/http"
	"net/url"
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

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var b strings.Builder
	b.WriteString(`<div class="wx-page">`)
	b.WriteString(searchForm(q))

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

func searchForm(q string) string {
	return `<form class="wx-find" method="GET" action="/weather">
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
	return serverCard(f, auth.PlaceName(accountID))
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
		b.WriteString(serverCard(f, first.Label()))
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

const pageCSS = `<style>
.wx-page{max-width:760px}
.wx-find{display:flex;gap:8px;margin:0 0 var(--spacing-lg,24px);flex-wrap:wrap}
.wx-find .field{flex:1;min-width:0}
.wx-note{color:var(--text-secondary,#555);line-height:1.6;margin:0}
.wx-also{margin:var(--spacing-lg,24px) 0 0;display:flex;align-items:baseline;gap:6px;flex-wrap:wrap}
.wx-also-head{font-size:12px;color:var(--text-muted,#999);text-transform:uppercase;letter-spacing:.04em}
</style>`
