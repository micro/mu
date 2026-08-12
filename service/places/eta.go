package places

// How long it takes to get somewhere.
//
// places could already tell an agent what is nearby and where it is, but not
// whether it is worth going — and "is it worth going" is the question a person
// actually asks. Distance as the crow flies does not answer it: a mile across a
// river is not a mile along a road.
//
// This uses the Google Routes API, which the same GOOGLE_API_KEY already
// reaches, and falls back to a straight-line estimate when there is no key so a
// self-hosted instance without Google still answers something honest — labelled
// as an estimate rather than passed off as a route.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
)

// googleRoutesURL is a var so a test can point it at a stub; the Routes API is
// not something to call for real from a test suite.
var googleRoutesURL = "https://routes.googleapis.com/directions/v2:computeRoutes"

var routesClient = &http.Client{Timeout: 15 * time.Second}

// travelModes maps the words a person uses to the Routes API's own.
var travelModes = map[string]string{
	"drive": "DRIVE", "driving": "DRIVE", "car": "DRIVE",
	"walk": "WALK", "walking": "WALK", "foot": "WALK",
	"cycle": "BICYCLE", "cycling": "BICYCLE", "bike": "BICYCLE", "bicycle": "BICYCLE",
	"transit": "TRANSIT", "bus": "TRANSIT", "train": "TRANSIT", "public": "TRANSIT",
}

// averageSpeedKPH is the fallback estimate per mode, used only when there is no
// Google key. Deliberately conservative: an estimate that reads slightly long is
// less damaging than one that makes someone late.
var averageSpeedKPH = map[string]float64{
	"DRIVE": 40, "WALK": 4.5, "BICYCLE": 15, "TRANSIT": 25,
}

// route is one leg's answer.
type route struct {
	Duration time.Duration
	// Static is the same journey with no traffic on the road. Zero when the
	// provider did not give one — a walk has no traffic, and an estimate has no
	// idea. The interesting number is the difference.
	Static   time.Duration
	Metres   int
	Estimate bool // true when this is a straight-line guess, not a real route
}

// Delay is how much of the journey is traffic.
func (r route) Delay() time.Duration {
	if r.Static <= 0 || r.Duration <= r.Static {
		return 0
	}
	return r.Duration - r.Static
}

// when says when the journey happens. Both zero means now, which is what every
// call to this used to mean and could not say otherwise.
type when struct {
	Depart time.Time // leave at this time
	Arrive time.Time // be there by this time
}

// computeRoute asks Google how long the journey takes.
func computeRoute(fromLat, fromLon, toLat, toLon float64, mode string, w when) (route, error) {
	key := googleAPIKey()
	if key == "" {
		return estimateRoute(fromLat, fromLon, toLat, toLon, mode), nil
	}

	payload := map[string]any{
		"origin":      latLng(fromLat, fromLon),
		"destination": latLng(toLat, toLon),
		"travelMode":  mode,
	}
	// Traffic-aware routing is only valid for driving; sending it for a walk is
	// rejected outright rather than ignored.
	if mode == "DRIVE" {
		payload["routingPreference"] = "TRAFFIC_AWARE"
	}
	// When the journey happens.
	//
	// Everything here used to mean "now", which is wrong for most of the
	// questions people actually ask — "how long on Monday morning" and "when do
	// I need to leave" are both unanswerable without it, and transit without a
	// departure time is a timetable read at the wrong hour.
	//
	// A time in the past is refused by the provider rather than ignored, and a
	// caller who says "8am" on a day that has already had one means tomorrow's
	// question, not an error. Dropped instead: the answer is then "now", which
	// is the answer they used to get anyway.
	switch {
	case !w.Depart.IsZero() && w.Depart.After(time.Now()):
		payload["departureTime"] = w.Depart.UTC().Format(time.RFC3339)
	case !w.Arrive.IsZero() && w.Arrive.After(time.Now()):
		if mode == "TRANSIT" {
			payload["arrivalTime"] = w.Arrive.UTC().Format(time.RFC3339)
		} else {
			// Only transit can be asked to arrive by a time. For everything else
			// the useful approximation is to ask about the road as it will be at
			// that hour, and subtract — which is what ETA does with the answer.
			payload["departureTime"] = w.Arrive.UTC().Format(time.RFC3339)
		}
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, googleRoutesURL, bytes.NewReader(body))
	if err != nil {
		return route{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	// staticDuration is the same journey with an empty road. It costs nothing
	// extra to ask for alongside duration, and it is the difference between
	// "34 minutes" and "34 minutes, 11 of them traffic" — which is the part
	// somebody deciding when to leave actually needs.
	req.Header.Set("X-Goog-FieldMask", "routes.duration,routes.distanceMeters,routes.staticDuration")

	resp, err := routesClient.Do(req)
	if err != nil {
		return estimateRoute(fromLat, fromLon, toLat, toLon, mode), nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// A key that reaches Places does not necessarily have Routes enabled,
		// which is a 403 that says nothing useful to whoever asked how long the
		// journey takes. Answer with the estimate and say it is one; the
		// operator can read the real reason in the log.
		app.Log("places", "routes api unavailable (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		return estimateRoute(fromLat, fromLon, toLat, toLon, mode), nil
	}

	var parsed struct {
		Routes []struct {
			Duration       string `json:"duration"`
			StaticDuration string `json:"staticDuration"`
			DistanceMeters int    `json:"distanceMeters"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return estimateRoute(fromLat, fromLon, toLat, toLon, mode), nil
	}
	if len(parsed.Routes) == 0 {
		// No route between these two points by this mode — an island by car,
		// say. That is an answer, not a failure of ours.
		return route{}, fmt.Errorf("no %s route between those places", strings.ToLower(mode))
	}

	d, err := time.ParseDuration(parsed.Routes[0].Duration)
	if err != nil {
		return estimateRoute(fromLat, fromLon, toLat, toLon, mode), nil
	}
	// A missing or unparseable staticDuration is not a failure — it means this
	// mode has no traffic to report. Zero, and Delay says nothing.
	static, _ := time.ParseDuration(parsed.Routes[0].StaticDuration)
	return route{Duration: d, Static: static, Metres: parsed.Routes[0].DistanceMeters}, nil
}

func latLng(lat, lon float64) map[string]any {
	return map[string]any{"location": map[string]any{
		"latLng": map[string]float64{"latitude": lat, "longitude": lon}}}
}

// estimateRoute is the no-key fallback: straight-line distance at an average
// speed for the mode, padded for the fact that roads are not straight.
func estimateRoute(fromLat, fromLon, toLat, toLon float64, mode string) route {
	km := haversineKM(fromLat, fromLon, toLat, toLon)
	// Real routes are longer than the line between their ends. 1.3 is the usual
	// rule of thumb for road networks.
	km *= 1.3
	speed := averageSpeedKPH[mode]
	if speed == 0 {
		speed = averageSpeedKPH["DRIVE"]
	}
	return route{
		Duration: time.Duration(km / speed * float64(time.Hour)),
		Metres:   int(km * 1000),
		Estimate: true,
	}
}

// haversineKM is the great-circle distance between two points.
func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthKM = 6371
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// travelMode resolves the caller's word to a Routes travel mode, and says
// whether it recognised it.
//
// It used to answer DRIVE to anything it did not know, so "by helicopter" and
// "by ferry" came back as confident driving times and nothing anywhere said the
// question had been changed. An unknown mode is now the caller's to correct:
// silently answering a different question is worse than refusing this one.
// Empty is still DRIVE, because that is the documented default rather than a
// guess.
func travelMode(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "DRIVE", true
	}
	m, ok := travelModes[s]
	return m, ok
}

// modeWords is every word travelMode accepts, for saying so when it refuses.
func modeWords() string {
	seen := map[string]bool{}
	var out []string
	for _, canonical := range []string{"drive", "walk", "cycle", "transit"} {
		if !seen[canonical] {
			seen[canonical] = true
			out = append(out, canonical)
		}
	}
	return strings.Join(out, ", ")
}

// journeyTime reads the caller's two optional times.
//
// Both together is a contradiction rather than a preference — "leave at 8 and
// be there by 9" is either a question about a journey or an assertion about
// one, and guessing which is how you answer neither.
func journeyTime(departAt, arriveBy string) (when, error) {
	departAt, arriveBy = strings.TrimSpace(departAt), strings.TrimSpace(arriveBy)
	if departAt != "" && arriveBy != "" {
		return when{}, fmt.Errorf("say either when you are leaving or when you need to arrive, not both")
	}
	var w when
	var err error
	if departAt != "" {
		if w.Depart, err = time.Parse(time.RFC3339, departAt); err != nil {
			return when{}, fmt.Errorf("I could not read %q as a time — use a full date and time like 2026-08-13T08:00:00Z", departAt)
		}
	}
	if arriveBy != "" {
		if w.Arrive, err = time.Parse(time.RFC3339, arriveBy); err != nil {
			return when{}, fmt.Errorf("I could not read %q as a time — use a full date and time like 2026-08-13T15:00:00Z", arriveBy)
		}
	}
	return w, nil
}

// humanDuration writes a duration the way a person says it: "22 min", "1 hr 5 min".
func humanDuration(d time.Duration) string {
	// Truncate rather than round: 30 seconds is "under a minute", and rounding
	// it up to "1 min" claims a precision the number does not have.
	mins := int(d.Truncate(time.Minute).Minutes())
	if mins < 1 {
		return "under a minute"
	}
	if mins < 60 {
		return fmt.Sprintf("%d min", mins)
	}
	h, m := mins/60, mins%60
	if m == 0 {
		return fmt.Sprintf("%d hr", h)
	}
	return fmt.Sprintf("%d hr %d min", h, m)
}

// humanDistance writes metres as km or m, whichever a person would say.
func humanDistance(metres int) string {
	if metres < 1000 {
		return fmt.Sprintf("%d m", metres)
	}
	return fmt.Sprintf("%.1f km", float64(metres)/1000)
}
