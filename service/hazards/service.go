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

// AlertsRequest narrows the alerts asked for.
type AlertsRequest struct {
	Level    string  `json:"level" description:"Lowest level to include: green, orange or red — default green (everything)"`
	Lat      float64 `json:"lat" description:"Optional: only alerts near this point"`
	Lon      float64 `json:"lon" description:"Optional: only alerts near this point"`
	WithinKm float64 `json:"within_km" description:"Optional: how near, in kilometres, default 1000 when lat/lon given"`
}

// AlertsResponse is what is happening.
type AlertsResponse struct {
	Text string `json:"text" description:"Current disasters, worst first, or nearest first when a point was given"`
}

// Alerts lists current disasters worldwide, from GDACS.
// @example {"level": "orange"}
func (Server) Alerts(_ context.Context, req *AlertsRequest, rsp *AlertsResponse) error {
	within := req.WithinKm
	if within <= 0 && (req.Lat != 0 || req.Lon != 0) {
		within = 1000
	}

	found, err := alerts(req.Level, req.Lat, req.Lon, within)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		if within > 0 {
			rsp.Text = fmt.Sprintf("No active disaster alerts within %.0fkm.", within)
			return nil
		}
		rsp.Text = "No active disaster alerts at that level."
		return nil
	}

	var b strings.Builder
	for i, a := range found {
		if i >= 15 {
			fmt.Fprintf(&b, "…and %d more.", len(found)-15)
			break
		}
		fmt.Fprintf(&b, "%s %s", a.Level, a.Kind)
		if a.Name != "" {
			fmt.Fprintf(&b, " %s", a.Name)
		}
		if a.Country != "" {
			fmt.Fprintf(&b, " — %s", a.Country)
		}
		if within > 0 {
			fmt.Fprintf(&b, ", %.0fkm away", a.AwayKm)
		}
		if a.Severity != "" {
			fmt.Fprintf(&b, "\n  %s", a.Severity)
		}
		b.WriteString("\n")
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
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
	Description: "What is going wrong in the physical world: earthquakes and disaster alerts, live",
	Page:        "/services/hazards",
	Icon:        "hazards.svg",
	Card:        service.Glance(Card),
	Endpoints: map[string]service.Endpoint{
		"Alerts": {
			Doc: "Current disasters worldwide from GDACS — cyclones, floods, volcanoes, " +
				"wildfires and earthquakes — with an alert level of green, orange or red. " +
				"Pass lat/lon to ask about somewhere in particular",
		},
		"Quakes": {
			Aliases: []string{"quakes"},
			Doc: "Recent earthquakes worldwide from the USGS, with magnitude, place and how " +
				"long ago. Pass lat/lon to ask about somewhere in particular, min for a " +
				"magnitude floor, and period for hour, day, week or month",
		},
		"Floods": {
			Doc: "Flood warnings and alerts in force in England, most severe first, from the " +
				"Environment Agency. The one hazard here that is a forecast rather than a " +
				"record — a warning says flooding is expected, which is something you can " +
				"act on. Optionally near a point. England only, and the answer says so",
		},
	},
}

// Handler sends /hazards to the page it has, which is its reference.
//
// It was a magnitude picker, a period picker, a table and a stylesheet: a form
// over hazards_quakes with one argument, drawn by hand. The card is the same
// answer and the tool takes the same argument, so the page had nothing of its
// own — and the derived page it got instead is /services/hazards, which is the
// card plus every method with its arguments and a form that calls them.
func Handler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, Spec.Page, http.StatusMovedPermanently)
}

func Card() string {
	quakes, err := recent(4.5, "day", 0, 0, 0)
	if err != nil {
		return ""
	}
	if len(quakes) == 0 {
		return `<p class="hmuted">Nothing above M4.5 in the past day.</p>` +
			`<p class="hmore"><a href="/hazards">Hazards →</a></p>`
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
	return b.String()
}

// trimF renders a magnitude without a trailing zero, so the links read M6+
// rather than M6.0+.
func trimF(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}
