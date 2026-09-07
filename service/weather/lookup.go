package weather

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/service"
)

// LookupRequest names a place rather than requiring coordinates.
type LookupRequest struct {
	Place string `json:"place" description:"Town or city to look up"`
	Focus string `json:"focus,omitempty" description:"rain for a rain-focused answer, otherwise a general forecast"`
}

func (Server) Lookup(ctx context.Context, req *LookupRequest, rsp *ForecastResponse) error {
	if len(req.Place) > 200 {
		return fmt.Errorf("place name is too long")
	}
	if req.Focus != "" && req.Focus != "rain" {
		return fmt.Errorf("focus must be rain or empty")
	}
	if strings.TrimSpace(req.Place) == "" {
		service.ServedFromCache(ctx)
		rsp.Summary = "Which town or city? Try ‘weather in London’."
		return nil
	}
	places, err := geocode(ctx, req.Place)
	if err != nil {
		return err
	}
	if len(places) == 0 {
		service.ServedFromCache(ctx)
		rsp.Summary = "I couldn't find that place. Please include the town or city and country."
		return nil
	}
	place := places[0]
	if !validCoordinates(place.Lat, place.Lon) {
		return fmt.Errorf("place lookup returned invalid coordinates")
	}
	forecast, err := FetchWeather(ctx, place.Lat, place.Lon)
	if err != nil {
		return err
	}
	if forecast == nil {
		return fmt.Errorf("forecast unavailable")
	}
	rsp.Summary = lookupForecastText(forecast, place.Label(), req.Focus, time.Now().UTC())
	return nil
}

// A direct answer is for a person, without the calendar instructions carried
// by the model-facing Forecast response. Dates remain explicit.
func lookupForecastText(f *WeatherForecast, place, focus string, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", place)
	today, haveToday := dailyItemForDate(f.DailyItems, at)
	if focus == "rain" {
		lead := "Today’s rain forecast is unavailable."
		if haveToday {
			if today.WillRain || today.RainMM > 0 || today.RainChance > 0 {
				lead = "Rain is possible today."
				if today.RainMM > 0 {
					lead = fmt.Sprintf("Rain is forecast today: %.1f mm.", today.RainMM)
				}
				if today.RainChance > 0 {
					lead += fmt.Sprintf(" Chance of rain: %d%%.", today.RainChance)
				}
			} else {
				lead = "No rain is indicated in today’s forecast."
			}
		}
		b.WriteString(lead + "\n\n")
	}
	if f.Current != nil && !f.ObservedAt.IsZero() {
		fmt.Fprintf(&b, "Now: %.0f°C, %s.\n", f.Current.TempC, f.Current.Description)
	} else {
		b.WriteString("Current conditions are unavailable.\n")
	}
	if !haveToday {
		b.WriteString("Today’s forecast is unavailable.\n")
	}
	shown := 0
	for _, day := range f.DailyItems {
		if day.Date.Format("2006-01-02") < at.Format("2006-01-02") {
			continue
		}
		fmt.Fprintf(&b, "\n%s: %.0f–%.0f°C, %s", day.Date.Format("Mon 2 Jan"), day.MinTempC, day.MaxTempC, day.Description)
		if day.RainChance > 0 {
			fmt.Fprintf(&b, ", %d%% chance of rain", day.RainChance)
		}
		if day.RainMM > 0 {
			fmt.Fprintf(&b, ", %.1f mm rain", day.RainMM)
		}
		b.WriteString(".\n")
		shown++
		if shown == 5 {
			break
		}
	}
	if !f.GeneratedAt.IsZero() {
		fmt.Fprintf(&b, "\nForecast updated %s UTC.", f.GeneratedAt.UTC().Format("2 Jan 15:04"))
	}
	return b.String()
}
