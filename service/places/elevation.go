package places

// How high somewhere is.
//
// Elevation is a fact about a location, which is what makes it a fourth
// endpoint here rather than a service of its own. There is no terrain domain a
// person would name — nobody goes looking for "the terrain section" — and a
// service that answered only this would be a single number wearing a nav item.
//
// The source is Open-Meteo, which weather already uses: a Copernicus digital
// elevation model, free, keyless, and global. It samples a 90-metre grid, so it
// answers "how high is this valley" well and "how high is this summit" to within
// a contour or two — Everest comes back a hundred metres under its surveyed
// height, because the highest point of a 90-metre cell is not the cell's average.
// Close enough for every question anybody actually asks of it, and worth saying
// out loud rather than presenting a grid sample as a survey.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// elevationBaseURL is a variable so tests can point it at a stub.
var elevationBaseURL = "https://api.open-meteo.com/v1/elevation"

var (
	elevationMu    sync.Mutex
	elevationCache = map[string]float64{}
)

// Elevation returns metres above sea level for a point.
//
// Cached without expiry, and permanently: the ground does not move on any
// timescale this cares about, and rounding the key to four decimal places makes
// two requests about the same hillside one lookup.
func Elevation(lat, lon float64) (float64, error) {
	key := fmt.Sprintf("%.4f,%.4f", lat, lon)

	elevationMu.Lock()
	if m, ok := elevationCache[key]; ok {
		elevationMu.Unlock()
		return m, nil
	}
	elevationMu.Unlock()

	url := fmt.Sprintf("%s?latitude=%.4f&longitude=%.4f", elevationBaseURL, lat, lon)
	rsp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		return 0, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("elevation: %s", rsp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, 1<<16))
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Elevation []float64 `json:"elevation"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if len(parsed.Elevation) == 0 {
		return 0, fmt.Errorf("elevation: no value for %s", key)
	}

	m := parsed.Elevation[0]
	elevationMu.Lock()
	if len(elevationCache) > 5000 {
		elevationCache = map[string]float64{}
	}
	elevationCache[key] = m
	elevationMu.Unlock()
	return m, nil
}

// describeElevation renders a height the way both halves of the world read it.
// Metres are the measurement; feet are what aviation, hiking maps and most of
// the people asking still use, so giving one without the other means half the
// callers have to convert.
func describeElevation(label string, lat, lon, metres float64) string {
	feet := metres * 3.28084
	switch {
	case metres < 0:
		return fmt.Sprintf("%s (%.4f, %.4f) is %.0f m (%.0f ft) below sea level.",
			label, lat, lon, -metres, -feet)
	case metres < 1:
		return fmt.Sprintf("%s (%.4f, %.4f) is at sea level.", label, lat, lon)
	default:
		return fmt.Sprintf("%s (%.4f, %.4f) is %.0f m (%.0f ft) above sea level.",
			label, lat, lon, metres, feet)
	}
}
