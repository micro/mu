// Package hazards is what is going wrong in the physical world right now.
//
// Earthquakes today, from the USGS feeds. Floods, volcanoes and severe-weather
// alerts are the same shape and would go behind more methods here rather than
// into services of their own — the domain is "something dangerous is happening",
// not "seismology".
//
// It needs no key, which is why it is worth building. Every tool behind an
// Atlas or Twilio account makes a self-hosted instance a little more hollow;
// this one works the moment somebody clones the repository, forever, for
// nothing. Public hazard data is published precisely so that anybody can
// redistribute it.
//
// The data is authoritative rather than plausible, which is the point of it. A
// model asked whether there has been an earthquake near Tokyo will produce a
// confident paragraph from its training data; this produces the actual list,
// with magnitudes and times, from the people who run the seismometers.
package hazards

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// usgsBase is the USGS real-time feed directory.
const usgsBase = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/"

// userAgent identifies this instance. TfL taught us that Go's default string is
// refused by at least one public service, and naming yourself is good manners
// to anybody running a free feed.
const userAgent = "mu/1.0 (+https://github.com/micro/mu)"

var client = &http.Client{Timeout: 20 * time.Second}

// quake is one event.
type quake struct {
	Magnitude float64
	Place     string
	When      time.Time
	Lat, Lon  float64
	Depth     float64
	Tsunami   bool
	URL       string
	AwayKm    float64 // filled when a caller asked about somewhere
}

// feedFor picks the USGS file covering a magnitude floor and a window.
//
// USGS publishes fixed combinations rather than a query API, so the job is to
// choose the smallest file that still contains everything asked for and filter
// the rest here. Asking for M3 over a day fetches the 2.5 feed and drops what
// is under three, which is one request instead of a search.
func feedFor(minMag float64, period string) (string, error) {
	var mag string
	switch {
	case minMag <= 1.0:
		mag = "1.0"
	case minMag <= 2.5:
		mag = "2.5"
	case minMag <= 4.5:
		mag = "4.5"
	default:
		mag = "significant"
	}

	switch strings.ToLower(strings.TrimSpace(period)) {
	case "", "day":
		period = "day"
	case "hour":
		period = "hour"
	case "week":
		period = "week"
	case "month":
		period = "month"
	default:
		return "", fmt.Errorf("period must be hour, day, week or month")
	}
	return mag + "_" + period + ".geojson", nil
}

// recent fetches events, filtered by magnitude and optionally by distance from
// a point.
func recent(minMag float64, period string, lat, lon float64, withinKm float64) ([]quake, error) {
	file, err := feedFor(minMag, period)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, usgsBase+file, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("earthquake data is unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("earthquake data returned %d", resp.StatusCode)
	}

	var raw struct {
		Features []struct {
			Properties struct {
				Mag     float64 `json:"mag"`
				Place   string  `json:"place"`
				Time    int64   `json:"time"` // milliseconds
				Tsunami int     `json:"tsunami"`
				URL     string  `json:"url"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"` // lon, lat, depth
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("could not read the earthquake feed: %w", err)
	}

	located := withinKm > 0 && (lat != 0 || lon != 0)
	out := make([]quake, 0, len(raw.Features))
	for _, f := range raw.Features {
		p := f.Properties
		if p.Mag < minMag {
			continue
		}
		q := quake{
			Magnitude: p.Mag,
			Place:     p.Place,
			When:      time.UnixMilli(p.Time).UTC(),
			Tsunami:   p.Tsunami == 1,
			URL:       p.URL,
		}
		if c := f.Geometry.Coordinates; len(c) >= 3 {
			q.Lon, q.Lat, q.Depth = c[0], c[1], c[2]
		}
		if located {
			q.AwayKm = haversineKm(lat, lon, q.Lat, q.Lon)
			if q.AwayKm > withinKm {
				continue
			}
		}
		out = append(out, q)
	}

	// Nearest first when somewhere was named, newest first otherwise — the
	// order answers the question that was actually asked.
	if located {
		sort.Slice(out, func(i, j int) bool { return out[i].AwayKm < out[j].AwayKm })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	}
	return out, nil
}

// haversineKm is the great-circle distance between two points.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ago renders how long since an event in the way somebody would say it.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
