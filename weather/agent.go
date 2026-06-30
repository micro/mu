package weather

import (
	"fmt"
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

	var sb strings.Builder
	fmt.Fprintf(&sb, "Current request date: %s.\n", time.Now().UTC().Format("Monday, 2 January 2006 (2006-01-02, UTC)"))
	if wf.Location != "" {
		fmt.Fprintf(&sb, "Weather for %s.\n", wf.Location)
	}
	if c := wf.Current; c != nil {
		fmt.Fprintf(&sb, "Now: %.0f°C (feels %.0f°C), %s, humidity %d%%, wind %.0f km/h.\n",
			c.TempC, c.FeelsLikeC, c.Description, c.Humidity, c.WindKph)
	}
	if len(wf.DailyItems) > 0 {
		sb.WriteString("Next days:\n")
		for i, d := range wf.DailyItems {
			if i >= 5 {
				break
			}
			rain := ""
			if d.WillRain || d.RainMM > 0 {
				rain = fmt.Sprintf(", rain %.0fmm", d.RainMM)
			}
			fmt.Fprintf(&sb, "%s: %.0f–%.0f°C, %s%s\n",
				d.Date.Format("Mon 2 Jan"), d.MinTempC, d.MaxTempC, d.Description, rain)
		}
	}
	if sb.Len() == 0 {
		return weatherUnavailableMessage
	}
	return sb.String()
}

func validCoordinates(lat, lon float64) bool {
	return lat == lat && lon == lon &&
		lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
