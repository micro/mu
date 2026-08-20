package weather

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for weather. Its methods are exposed
// as RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// ForecastRequest is the input for a forecast lookup.
type ForecastRequest struct {
	Lat float64 `json:"lat" required:"true" description:"Latitude of the location"`
	Lon float64 `json:"lon" required:"true" description:"Longitude of the location"`
}

// ForecastResponse is a model-ready weather summary.
type ForecastResponse struct {
	Summary string `json:"summary" description:"Current conditions plus the next few days"`
}

// Forecast returns the weather forecast for a location (current conditions
// plus the next few days).
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Forecast(ctx context.Context, req *ForecastRequest, rsp *ForecastResponse) error {
	// ctx carries the call's meter: a forecast served from the shared cache
	// costs this instance nothing and is not charged for. See FetchWeather.
	rsp.Summary = ForecastText(ctx, req.Lat, req.Lon)
	return nil
}

// AirRequest is the input for an air quality lookup.
type AirRequest struct {
	Lat float64 `json:"lat" required:"true" description:"Latitude of the location"`
	Lon float64 `json:"lon" required:"true" description:"Longitude of the location"`
}

// AirResponse is a model-ready air quality summary.
type AirResponse struct {
	Summary string `json:"summary" description:"Air quality index, pollutants, UV and pollen where it is measured"`
}

// Air returns current air quality, UV and pollen for a location.
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Air(_ context.Context, req *AirRequest, rsp *AirResponse) error {
	a, err := airQuality(req.Lat, req.Lon)
	if err != nil {
		return err
	}

	var b strings.Builder
	if a.HaveEuropeanAQI {
		fmt.Fprintf(&b, "Air quality %s — European AQI %d", aqiWord(a.EuropeanAQI), a.EuropeanAQI)
		if a.HaveUSAQIReading {
			fmt.Fprintf(&b, ", US AQI %d", a.USAQI)
		}
		b.WriteString("\n")
	} else if a.HaveUSAQIReading {
		fmt.Fprintf(&b, "US AQI %d\n", a.USAQI)
	}
	fmt.Fprintf(&b, "PM2.5 %.1f, PM10 %.1f, ozone %.0f, NO₂ %.1f µg/m³\n",
		a.PM25, a.PM10, a.Ozone, a.NO2)
	fmt.Fprintf(&b, "UV index %.1f", a.UV)

	// Only the pollens actually in the air, and only where somebody counts
	// them — a list of six zeroes is noise, and no list at all is a fact.
	if a.HavePollen {
		var up []string
		for _, name := range []string{"grass", "birch", "alder", "olive", "mugwort", "ragweed"} {
			if v, ok := a.Pollen[name]; ok && v >= 1 {
				up = append(up, fmt.Sprintf("%s %s", name, pollenWord(v)))
			}
		}
		if len(up) > 0 {
			fmt.Fprintf(&b, "\nPollen: %s", strings.Join(up, ", "))
		} else {
			b.WriteString("\nPollen: none of consequence")
		}
	}
	rsp.Summary = b.String()
	return nil
}

// MarineRequest is the input for a sea state lookup.
type MarineRequest struct {
	Lat  float64 `json:"lat" required:"true" description:"Latitude of a coastal or offshore point"`
	Lon  float64 `json:"lon" required:"true" description:"Longitude of a coastal or offshore point"`
	Days int     `json:"days" description:"How many days ahead, 1 to 7 — default 3"`
}

// MarineResponse is a model-ready sea state summary.
type MarineResponse struct {
	Summary string `json:"summary" description:"Wave height, period and direction now and the days ahead"`
}

// Marine returns wave conditions at a coastal point and the days ahead.
// @example {"lat": 50.35, "lon": -4.14}
func (Server) Marine(_ context.Context, req *MarineRequest, rsp *MarineResponse) error {
	s, err := sea(req.Lat, req.Lon, req.Days)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Waves %.1fm at %.0fs from the %s",
		s.WaveHeight, s.WavePeriod, compass(s.WaveDirection))
	if s.SwellHeight > 0 {
		fmt.Fprintf(&b, ", swell %.1fm at %.0fs", s.SwellHeight, s.SwellPeriod)
	}
	if s.HaveSeaTemp {
		fmt.Fprintf(&b, ", sea %.1f°C", s.SeaTempC)
	}
	b.WriteString("\n")
	for _, d := range s.Days {
		fmt.Fprintf(&b, "%s  up to %.1fm at %.0fs from the %s\n",
			d.Date.Format("Mon 2 Jan"), d.MaxHeight, d.MaxPeriod, compass(d.DominantFrom))
	}
	rsp.Summary = strings.TrimRight(b.String(), "\n")
	return nil
}

// HistoryRequest is the input for a past weather lookup.
type HistoryRequest struct {
	Lat   float64 `json:"lat" required:"true" description:"Latitude of the location"`
	Lon   float64 `json:"lon" required:"true" description:"Longitude of the location"`
	Start string  `json:"start" required:"true" description:"First day, as 2025-08-01"`
	End   string  `json:"end" required:"true" description:"Last day, as 2025-08-31"`
}

// HistoryResponse is a model-ready summary of recorded weather.
type HistoryResponse struct {
	Summary string `json:"summary" description:"What the weather actually was: averages, totals and the extremes"`
}

// History returns recorded weather for a location over a date range.
// @example {"lat": 51.5074, "lon": -0.1278, "start": "2025-08-01", "end": "2025-08-31"}
func (Server) History(_ context.Context, req *HistoryRequest, rsp *HistoryResponse) error {
	p, err := past(req.Lat, req.Lon, req.Start, req.End)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d days from %s to %s\n",
		len(p.Days), p.Days[0].Date.Format("2 Jan 2006"), p.Days[len(p.Days)-1].Date.Format("2 Jan 2006"))
	fmt.Fprintf(&b, "Average high %.1f°C, average low %.1f°C\n", p.MeanMaxC, p.MeanMinC)
	fmt.Fprintf(&b, "Total rainfall %.1fmm\n", p.TotalRain)
	fmt.Fprintf(&b, "Warmest %.1f°C on %s", p.HottestC, p.Hottest.Format("2 Jan"))
	if p.WettestMM > 0 {
		fmt.Fprintf(&b, ", wettest %.1fmm on %s", p.WettestMM, p.Wettest.Format("2 Jan"))
	}

	// The day-by-day list is what somebody checking a specific date came for,
	// but a year of it is 365 lines nobody reads — so the summary above always
	// stands on its own and the detail only appears when it is short enough to
	// be read.
	if len(p.Days) <= 31 {
		b.WriteString("\n")
		for _, d := range p.Days {
			fmt.Fprintf(&b, "\n%s  %.1f°C / %.1f°C", d.Date.Format("Mon 2 Jan"), d.MaxC, d.MinC)
			if d.RainMM > 0 {
				fmt.Fprintf(&b, ", %.1fmm", d.RainMM)
			}
		}
	}
	rsp.Summary = b.String()
	return nil
}

var Spec = service.Spec{
	Name:        "weather",
	Handler:     new(Server),
	Description: "Forecast, air quality, sea state and what the weather actually was",
	Page:        "/weather",
	Icon:        "weather.svg",
	Card:        service.Personal(CardHTML),
	Endpoints: map[string]service.Endpoint{
		"Forecast": {Doc: "Get the weather forecast for a location — current conditions and the days ahead", Cost: quota.OpWeatherForecast},
		"Air": {Doc: "Air quality at a location right now — AQI, PM2.5, PM10, ozone, nitrogen dioxide, " +
			"UV index, and pollen where it is counted"},
		"Marine": {Doc: "Sea state at a coastal or offshore point — wave height, period and direction now " +
			"and for the days ahead, with sea temperature"},
		"History": {Doc: "What the weather actually was at a location between two dates — average high and " +
			"low, total rainfall and the extremes. Records run a few days behind today"},
	},
}
