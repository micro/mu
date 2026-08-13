package hazards

// The method, the page and the card.
//
// Priced at zero: USGS costs this instance nothing, and quota.json's rule is
// that a call is charged when it costs us something. Free also means an
// anonymous agent can ask, which is right for data published so that anybody
// can redistribute it — and right for the one category of tool where somebody
// might genuinely need an answer and not have an account.

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// QuakesRequest narrows what is asked for.
type QuakesRequest struct {
	Min      float64 `json:"min" description:"Smallest magnitude to include, default 2.5"`
	Period   string  `json:"period" description:"hour, day, week or month — default day"`
	Lat      float64 `json:"lat" description:"Optional: only events near this point"`
	Lon      float64 `json:"lon" description:"Optional: only events near this point"`
	WithinKm float64 `json:"within_km" description:"Optional: how near, in kilometres, default 500 when lat/lon given"`
}

// QuakesResponse is what happened.
type QuakesResponse struct {
	Text string `json:"text" description:"Recent earthquakes, nearest first when a point was given, newest first otherwise"`
}

// Quakes lists recent earthquakes worldwide, from the USGS.
// @example {"min": 4.5, "period": "day"}
func (Server) Quakes(_ context.Context, req *QuakesRequest, rsp *QuakesResponse) error {
	min := req.Min
	if min <= 0 {
		min = 2.5
	}
	within := req.WithinKm
	if within <= 0 && (req.Lat != 0 || req.Lon != 0) {
		within = 500
	}

	quakes, err := recent(min, req.Period, req.Lat, req.Lon, within)
	if err != nil {
		return err
	}
	if len(quakes) == 0 {
		if within > 0 {
			rsp.Text = fmt.Sprintf("No earthquakes of magnitude %.1f or above within %.0fkm in the last %s.",
				min, within, periodWord(req.Period))
			return nil
		}
		rsp.Text = fmt.Sprintf("No earthquakes of magnitude %.1f or above in the last %s.",
			min, periodWord(req.Period))
		return nil
	}

	var b strings.Builder
	for i, q := range quakes {
		if i >= 15 {
			fmt.Fprintf(&b, "…and %d more.", len(quakes)-15)
			break
		}
		fmt.Fprintf(&b, "M%.1f  %s — %s", q.Magnitude, q.Place, ago(q.When))
		if within > 0 {
			fmt.Fprintf(&b, ", %.0fkm away", q.AwayKm)
		}
		// A tsunami flag is the one field here somebody might act on, so it is
		// never buried in a depth figure nobody reads.
		if q.Tsunami {
			b.WriteString("  ⚠ tsunami warning issued")
		}
		b.WriteString("\n")
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

func periodWord(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "hour":
		return "hour"
	case "week":
		return "week"
	case "month":
		return "month"
	}
	return "day"
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("hazards", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "hazards",
	Handler:     new(Server),
	Description: "What is going wrong in the physical world: earthquakes, live from the USGS",
	Page:        "/hazards",
	Icon:        "hazards.svg",
	Card:        Card,
	Endpoints: map[string]service.Endpoint{
		"Quakes": {
			Aliases: []string{"quakes"},
			Doc: "Recent earthquakes worldwide from the USGS, with magnitude, place and how " +
				"long ago. Pass lat/lon to ask about somewhere in particular, min for a " +
				"magnitude floor, and period for hour, day, week or month",
		},
	},
}

// Handler serves /hazards.
func Handler(w http.ResponseWriter, r *http.Request) {
	min := 4.5
	if v := r.URL.Query().Get("min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			min = f
		}
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	var b strings.Builder
	b.WriteString(`<div class="hwrap"><div class="card"><h2>Hazards</h2>`)
	b.WriteString(`<p class="hlede">Earthquakes worldwide, live from the ` +
		`<a href="https://earthquake.usgs.gov" rel="noopener">USGS</a>. Free, needs no key, ` +
		`and callable by an agent — see <a href="/tools">Tools</a>.</p></div>`)

	b.WriteString(`<div class="card"><div class="hpick">`)
	for _, m := range []float64{2.5, 4.5, 6.0} {
		cls := "hopt"
		if m == min {
			cls += " hon"
		}
		b.WriteString(`<a class="` + cls + `" href="/hazards?min=` + trimF(m) +
			`&period=` + html.EscapeString(period) + `">M` + trimF(m) + `+</a>`)
	}
	for _, p := range []string{"day", "week"} {
		cls := "hopt"
		if p == period {
			cls += " hon"
		}
		b.WriteString(`<a class="` + cls + `" href="/hazards?min=` + trimF(min) +
			`&period=` + p + `">past ` + p + `</a>`)
	}
	b.WriteString(`</div>`)

	quakes, err := recent(min, period, 0, 0, 0)
	switch {
	case err != nil:
		b.WriteString(`<p class="hmuted">` + html.EscapeString(err.Error()) + `</p>`)
	case len(quakes) == 0:
		b.WriteString(`<p class="hmuted">Nothing at M` + trimF(min) + ` or above in the past ` +
			html.EscapeString(period) + `.</p>`)
	default:
		for i, q := range quakes {
			if i >= 40 {
				break
			}
			b.WriteString(`<div class="hq"><span class="hmag">M` +
				fmt.Sprintf("%.1f", q.Magnitude) + `</span> ` +
				`<a href="` + html.EscapeString(q.URL) + `" rel="noopener">` +
				html.EscapeString(q.Place) + `</a>` +
				`<span class="hago">` + html.EscapeString(ago(q.When)) + `</span>`)
			if q.Tsunami {
				b.WriteString(`<div class="htsu">⚠ tsunami warning issued</div>`)
			}
			b.WriteString(`</div>`)
		}
	}
	b.WriteString(`</div></div>` + pageStyle)

	w.Write([]byte(app.RenderHTMLForRequest("Hazards", //nolint:errcheck
		"Earthquakes worldwide, live from the USGS", b.String(), r)))
}

// Card renders the home-screen card: the largest few, recently.
func Card() string {
	quakes, err := recent(4.5, "day", 0, 0, 0)
	if err != nil {
		return ""
	}
	if len(quakes) == 0 {
		return `<p class="hmuted">Nothing above M4.5 in the past day.</p>` +
			`<p class="hmore"><a href="/hazards">Hazards →</a></p>` + pageStyle
	}
	var b strings.Builder
	for i, q := range quakes {
		if i >= 4 {
			break
		}
		b.WriteString(`<div class="hq"><span class="hmag">M` +
			fmt.Sprintf("%.1f", q.Magnitude) + `</span> ` +
			html.EscapeString(q.Place) +
			`<span class="hago">` + html.EscapeString(ago(q.When)) + `</span></div>`)
	}
	b.WriteString(`<p class="hmore"><a href="/hazards">All hazards →</a></p>`)
	return b.String() + pageStyle
}

// trimF renders a magnitude without a trailing zero, so the links read M6+
// rather than M6.0+.
func trimF(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

const pageStyle = `<style>
.hwrap{max-width:680px;margin:0 auto}
.hlede{color:#666;font-size:15px;margin:0}
.hmuted{color:#888;font-size:14px;margin:0}
.hpick{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:12px}
.hopt{border:1px solid var(--border-color,#e3e3e3);border-radius:var(--border-radius,8px);
  padding:6px 12px;font-size:14px;text-decoration:none;color:inherit}
.hopt.hon{border-color:#111;font-weight:600}
.hq{padding:8px 0;border-bottom:1px solid var(--border-color,#eee);font-size:15px;
  display:flex;align-items:baseline;gap:8px;flex-wrap:wrap}
.hq:last-of-type{border-bottom:0}
.hmag{font-variant-numeric:tabular-nums;font-weight:600;min-width:3.4em}
.hago{color:#888;font-size:13px;margin-left:auto}
.htsu{color:#b23;font-size:13px;width:100%}
.hmore{margin:12px 0 0;font-size:14px}
</style>`
