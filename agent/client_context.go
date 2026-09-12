package agent

import (
	"fmt"
	"math"
	"time"
)

// ClientContext is transient request data, never account or conversation state.
type ClientContext struct {
	Location *DeviceLocation `json:"location,omitempty"`
	Timezone string          `json:"timezone,omitempty"`
}
type DeviceLocation struct {
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Accuracy   float64   `json:"accuracy_m"`
	CapturedAt time.Time `json:"captured_at"`
	Source     string    `json:"source"`
}

func (c ClientContext) facts(now time.Time) string {
	result := ""
	if c.Timezone != "" && len(c.Timezone) <= 80 {
		if zone, err := time.LoadLocation(c.Timezone); err == nil {
			result = fmt.Sprintf("Client timezone: %s; local time %s. This is device-reported context for this request.\n", c.Timezone, now.In(zone).Format(time.RFC3339))
		}
	}
	l := c.Location
	if l == nil || l.Source != "device" || math.IsNaN(l.Latitude) || math.IsNaN(l.Longitude) || math.IsNaN(l.Accuracy) || math.IsInf(l.Latitude, 0) || math.IsInf(l.Longitude, 0) || math.IsInf(l.Accuracy, 0) || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180 || l.Accuracy < 0 || l.Accuracy > 50000 || l.CapturedAt.IsZero() || now.Sub(l.CapturedAt) > 5*time.Minute || l.CapturedAt.After(now.Add(30*time.Second)) {
		return result
	}
	// Round even if a third-party client sends greater precision.
	lat, lon := math.Round(l.Latitude*100)/100, math.Round(l.Longitude*100)/100
	accuracy := math.Max(l.Accuracy, 1600)
	return result + fmt.Sprintf("Recent approximate device location, shared for this request: latitude %.2f, longitude %.2f; uncertainty at least %.0f metres; captured %s. Prefer an explicit place in the user's question; otherwise use this for here/near me/current location. Saved profile location is only a fallback. Do not infer a street address or exact venue, or save this position as memory/profile data.\n", lat, lon, accuracy, l.CapturedAt.UTC().Format(time.RFC3339))
}
