package weather

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ForecastText returns a compact, model-ready weather summary for a location.
// It is the AI-first accessor behind the weather_forecast agent tool.
func ForecastText(ctx context.Context, lat, lon float64) string {
	if !validCoordinates(lat, lon) {
		return "Weather is unavailable because the requested coordinates are invalid."
	}

	wf, err := FetchWeather(ctx, lat, lon)
	if err != nil || wf == nil {
		return weatherUnavailableMessage
	}

	return formatForecastText(wf, time.Now().UTC())
}

func formatForecastText(wf *WeatherForecast, now time.Time) string {
	if wf == nil {
		return weatherUnavailableMessage
	}

	now = now.UTC()
	var sb strings.Builder
	if wf.Location != "" {
		fmt.Fprintf(&sb, "Weather for %s.\n", wf.Location)
	}
	if c := wf.Current; c != nil && !wf.ObservedAt.IsZero() {
		details := []string{fmt.Sprintf("%.0f°C", c.TempC)}
		if c.FeelsLikeC != 0 || c.TempC == 0 {
			details = append(details, fmt.Sprintf("feels %.0f°C", c.FeelsLikeC))
		}
		if c.Description != "" {
			details = append(details, c.Description)
		}
		if c.HumidityAvailable {
			details = append(details, "humidity "+strconv.Itoa(c.Humidity)+"%")
		}
		if c.WindKphAvailable {
			details = append(details, fmt.Sprintf("wind %.0f km/h", c.WindKph))
		}
		if !c.HumidityAvailable || !c.WindKphAvailable {
			details = append(details, "some current observations unavailable")
		}
		fmt.Fprintf(&sb, "Now: %s.\n", strings.Join(details, ", "))
	}
	if d, ok := dailyItemForDate(wf.DailyItems, now); ok {
		rain := ""
		if d.WillRain || d.RainMM > 0 {
			rain = fmt.Sprintf(", rain %.0fmm", d.RainMM)
		}
		fmt.Fprintf(&sb, "Today: %s: %.0f–%.0f°C, %s%s.\n", d.Date.Format("Monday, 2 January 2006 (2006-01-02)"), d.MinTempC, d.MaxTempC, d.Description, rain)
		if line := lightToday(d, now); line != "" {
			sb.WriteString(line + "\n")
		}
	} else if len(wf.DailyItems) > 0 {
		fmt.Fprintf(&sb, "Today: forecast unavailable for the request date %s.\n", now.Format("Monday, 2 January 2006 (2006-01-02)"))
	}
	source := strings.TrimSpace(wf.Source)
	if source == "" {
		source = "weather provider"
	}
	generatedAt := wf.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = now
	}
	fmt.Fprintf(&sb, "Current request date: %s.\n", now.Format("Monday, 2 January 2006 (2006-01-02, UTC)"))
	sb.WriteString("Calendar rule: anchor relative words like today/tomorrow to the request date above, use only the dated forecast rows below, and do not invent dates. If a requested day is not listed, say it is unavailable.\n")
	fmt.Fprintf(&sb, "Freshness/source: source %s; generated at %s.\n", source, generatedAt.Format("2006-01-02 15:04 UTC"))
	if wf.ObservedAt.IsZero() {
		sb.WriteString("Current conditions status: unavailable — provider did not return a current/hourly observation timestamp, so do not present a current-weather claim.\n")
	} else {
		fmt.Fprintf(&sb, "Current conditions observed at %s.\n", wf.ObservedAt.UTC().Format("2006-01-02 15:04 UTC"))
	}
	if len(wf.DailyItems) > 0 {
		sb.WriteString("Dated daily forecast (provider timestamps):\n")
		for i, d := range wf.DailyItems {
			if i >= 5 {
				break
			}
			rain := ""
			if d.WillRain || d.RainMM > 0 {
				rain = fmt.Sprintf(", rain %.0fmm", d.RainMM)
			}
			fmt.Fprintf(&sb, "%s: %.0f–%.0f°C, %s%s\n",
				d.Date.Format("Monday, 2 January 2006 (2006-01-02)"), d.MinTempC, d.MaxTempC, d.Description, rain)
		}
	}
	if len(wf.DailyItems) == 0 && (wf.Current == nil || wf.ObservedAt.IsZero()) {
		return weatherUnavailableMessage
	}
	return sb.String()
}

func dailyItemForDate(items []DailyItem, date time.Time) (DailyItem, bool) {
	want := date.UTC().Format("2006-01-02")
	for _, item := range items {
		if item.Date.UTC().Format("2006-01-02") == want {
			return item, true
		}
	}
	return DailyItem{}, false
}

func validCoordinates(lat, lon float64) bool {
	return lat == lat && lon == lon &&
		lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// lightToday is when the sun rises and sets, and how much of it is left.
//
// The remaining-light part is the point. For anything outdoors it is the fact
// that decides the afternoon — "18°C and cloudy" does not tell you whether to
// set off and "the light goes in forty minutes" does — and it is arithmetic
// over a field the provider was already returning and this was parsing past.
//
// Nothing at all where the sun does neither, which is a real place for part of
// the year rather than an error, and nothing after dark: "0 minutes of daylight
// left" at ten at night is a true sentence nobody needs.
func lightToday(d DailyItem, now time.Time) string {
	if d.Sunrise.IsZero() && d.Sunset.IsZero() {
		return ""
	}
	var parts []string
	if !d.Sunrise.IsZero() {
		parts = append(parts, "sunrise "+d.Sunrise.Format("15:04")+" UTC")
	}
	if !d.Sunset.IsZero() {
		parts = append(parts, "sunset "+d.Sunset.Format("15:04")+" UTC")
	}
	line := "Light: " + strings.Join(parts, ", ")

	if !d.Sunset.IsZero() && now.Before(d.Sunset) && (d.Sunrise.IsZero() || now.After(d.Sunrise)) {
		left := d.Sunset.Sub(now).Round(time.Minute)
		switch {
		case left >= time.Hour:
			line += fmt.Sprintf(" — %.0fh %.0fm of daylight left", left.Hours(),
				left.Minutes()-float64(int(left.Hours()))*60)
		default:
			line += fmt.Sprintf(" — %.0f minutes of daylight left", left.Minutes())
		}
	}
	return line + "."
}
