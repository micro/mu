package weather

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ForecastText returns a compact, model-ready weather summary for a location.
// It is the AI-first accessor behind the weather_forecast agent tool.
func ForecastText(lat, lon float64) string {
	if !validCoordinates(lat, lon) {
		return "Weather is unavailable because the requested coordinates are invalid."
	}

	wf, err := FetchWeather(lat, lon)
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
	fmt.Fprintf(&sb, "Current request date: %s.\n", now.Format("Monday, 2 January 2006 (2006-01-02, UTC)"))
	sb.WriteString("Calendar rule: anchor relative words like today/tomorrow to the request date above, use only the dated forecast rows below, and do not invent dates. If a requested day is not listed, say it is unavailable.\n")
	if wf.Location != "" {
		fmt.Fprintf(&sb, "Weather for %s.\n", wf.Location)
	}
	source := strings.TrimSpace(wf.Source)
	if source == "" {
		source = "weather provider"
	}
	generatedAt := wf.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = now
	}
	fmt.Fprintf(&sb, "Freshness/source: source %s; generated at %s.\n", source, generatedAt.Format("2006-01-02 15:04 UTC"))
	if wf.ObservedAt.IsZero() {
		sb.WriteString("Current conditions status: unavailable — provider did not return a current/hourly observation timestamp, so do not present a current-weather claim.\n")
	} else {
		fmt.Fprintf(&sb, "Current conditions observed at %s.\n", wf.ObservedAt.UTC().Format("2006-01-02 15:04 UTC"))
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

func validCoordinates(lat, lon float64) bool {
	return lat == lat && lon == lon &&
		lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
