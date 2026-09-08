package weather

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/service"
)

// LookupRequest names a place rather than requiring coordinates.
type LookupRequest struct {
	Day   string `json:"day,omitempty" description:"today or tomorrow; empty returns the current multi-day forecast"`
	Place string `json:"place" description:"Town or city to look up"`
	Focus string `json:"focus,omitempty" description:"rain for a rain-focused answer, otherwise a general forecast"`
}

func (Server) Lookup(ctx context.Context, req *LookupRequest, rsp *ForecastResponse) error {
	if req.Day != "" && req.Day != "today" && req.Day != "tomorrow" {
		return fmt.Errorf("day must be today or tomorrow")
	}
	if len(req.Place) > 200 {
		return fmt.Errorf("place name is too long")
	}
	if req.Focus != "" && req.Focus != "rain" {
		return fmt.Errorf("focus must be rain or empty")
	}
	place := Place{}
	named := strings.TrimSpace(req.Place)
	owner := service.AccountFrom(ctx)
	if named == "" {
		if lat, lon, ok := auth.Located(owner); ok {
			place = Place{Name: auth.PlaceName(owner), Lat: lat, Lon: lon}
			if place.Name == "" {
				place.Name = "Your location"
			}
		} else {
			named = strings.TrimSpace(auth.PlaceName(owner))
		}
	}
	if place.Name == "" {
		if named == "" {
			service.ServedFromCache(ctx)
			rsp.Summary = "Which town or city? Try ‘weather in London’."
			return nil
		}
		places, err := geocode(ctx, named)
		if err != nil {
			return err
		}
		if len(places) == 0 {
			service.ServedFromCache(ctx)
			rsp.Summary = "I couldn't find that place. Please include the town or city and country."
			return nil
		}
		place = places[0]
	}
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
	at := localLookupTime(owner)
	if req.Day == "tomorrow" {
		at = at.AddDate(0, 0, 1)
	}
	if req.Day != "" {
		rsp.Summary = lookupDayText(forecast, place.Label(), at)
	} else {
		rsp.Summary = lookupForecastText(forecast, place.Label(), req.Focus, at)
	}
	return nil
}

func localLookupTime(owner string) time.Time {
	now := time.Now().UTC()
	if acc, err := auth.GetAccount(owner); owner != "" && err == nil && acc != nil {
		if loc, err := time.LoadLocation(acc.Zone); err == nil {
			return now.In(loc)
		}
	}
	return now
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

// Date words are declared by the service that can honour them, not stripped
// globally by the command parser.
func lookupCommands(base []service.Command) []service.Command {
	out := append([]service.Command(nil), base...)
	for i := range out {
		if strings.HasSuffix(out[i].Pattern, " today") {
			out[i].Defaults = map[string]any{"day": "today"}
		}
	}
	for _, day := range []string{"today", "tomorrow"} {
		for _, pattern := range []string{"weather", "weather in {place}", "weather {place}", "will it rain", "will it rain in {place}"} {
			out = append(out, service.Command{Pattern: pattern, Suffix: day, Defaults: map[string]any{"day": day}})
		}
		out = append(out, service.Command{Pattern: "weather " + day + " in {place}", Defaults: map[string]any{"day": day}})
	}
	return out
}

func lookupDayText(f *WeatherForecast, place string, at time.Time) string {
	day, ok := dailyItemForDate(f.DailyItems, at)
	label := at.Format("Monday 2 January")
	if !ok {
		return fmt.Sprintf("%s\n\nThe forecast for %s is unavailable.", place, label)
	}
	return fmt.Sprintf("%s\n\n%s: %.0f–%.0f°C, %s. Rain: %.1f mm; chance of rain: %d%%.", place, label, day.MinTempC, day.MaxTempC, day.Description, day.RainMM, day.RainChance)
}
