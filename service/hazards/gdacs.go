package hazards

// Global disaster alerts, from GDACS.
//
// The sibling of quakes rather than a replacement for it. USGS is the authority
// on earthquakes and reports every tremor worth a seismometer's attention;
// GDACS is the authority on whether something is a disaster, and covers the
// four kinds a seismometer cannot see — cyclones, floods, volcanoes and
// wildfires. Asking "has there been an earthquake" and asking "is anywhere in
// trouble" are different questions.
//
// Also keyless, and also published to be redistributed: GDACS exists so that
// responders and the public can act on it.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// gdacsURL is the current event list, all hazard types.
const gdacsURL = "https://www.gdacs.org/gdacsapi/api/events/geteventlist/EVENTS4APP"

// alert is one ongoing or recent disaster.
type alert struct {
	Kind     string // human-readable: Earthquake, Cyclone…
	Level    string // Green, Orange, Red
	Name     string
	Country  string
	Severity string
	From     time.Time
	AwayKm   float64
	Lat, Lon float64
}

// kinds maps GDACS's two-letter codes to words.
//
// A caller should not have to learn that TC means tropical cyclone to
// understand an answer about one.
var kinds = map[string]string{
	"EQ": "Earthquake",
	"TC": "Cyclone",
	"FL": "Flood",
	"VO": "Volcano",
	"DR": "Drought",
	"WF": "Wildfire",
}

// levels orders severity so the worst sorts first.
var levels = map[string]int{"Red": 0, "Orange": 1, "Green": 2}

// alerts fetches current events, optionally filtered by level and location.
func alerts(minLevel string, lat, lon, withinKm float64) ([]alert, error) {
	req, err := http.NewRequest(http.MethodGet, gdacsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("disaster alerts are unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("disaster alerts returned %d", resp.StatusCode)
	}

	var raw struct {
		Features []struct {
			Properties struct {
				EventType  string `json:"eventtype"`
				EventName  string `json:"eventname"`
				AlertLevel string `json:"alertlevel"`
				Country    string `json:"country"`
				FromDate   string `json:"fromdate"`
				HTML       string `json:"htmldescription"`
				Severity   struct {
					Text string `json:"severitytext"`
				} `json:"severitydata"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("could not read the alert feed: %w", err)
	}

	floor, bounded := levels[strings.Title(strings.ToLower(strings.TrimSpace(minLevel)))] //nolint:staticcheck
	if strings.TrimSpace(minLevel) == "" {
		floor, bounded = levels["Green"], true
	}
	if !bounded {
		return nil, fmt.Errorf("level must be green, orange or red")
	}
	located := withinKm > 0 && (lat != 0 || lon != 0)

	out := make([]alert, 0, len(raw.Features))
	for _, f := range raw.Features {
		p := f.Properties
		rank, known := levels[p.AlertLevel]
		if !known || rank > floor {
			continue
		}
		a := alert{
			Kind:     kindWord(p.EventType),
			Level:    p.AlertLevel,
			Name:     strings.TrimSpace(p.EventName),
			Country:  strings.TrimSpace(p.Country),
			Severity: usefulSeverity(p.Severity.Text),
		}
		if t, err := time.Parse("2006-01-02T15:04:05", p.FromDate); err == nil {
			a.From = t
		}
		if c := f.Geometry.Coordinates; len(c) >= 2 {
			a.Lon, a.Lat = c[0], c[1]
		}
		if located {
			a.AwayKm = haversineKm(lat, lon, a.Lat, a.Lon)
			if a.AwayKm > withinKm {
				continue
			}
		}
		out = append(out, a)
	}

	sort.Slice(out, func(i, j int) bool {
		if located {
			return out[i].AwayKm < out[j].AwayKm
		}
		if levels[out[i].Level] != levels[out[j].Level] {
			return levels[out[i].Level] < levels[out[j].Level]
		}
		return out[i].From.After(out[j].From)
	})
	return out, nil
}

// usefulSeverity drops the severity line when it says nothing.
//
// GDACS reports "Magnitude 0" for a flood, which is not a magnitude and not
// zero of anything — printing it makes an alert look like a bug in us rather
// than a gap in the feed.
func usefulSeverity(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "Magnitude 0") {
		return ""
	}
	return s
}

func kindWord(code string) string {
	if w, ok := kinds[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return w
	}
	return code
}
