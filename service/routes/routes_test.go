package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Without a routing key the answer must still be honest: an estimate, labelled
// as one. A self-hosted instance with no Google key should get something useful
// rather than an error, but it must not read like a real route.
func TestETAWithoutAKeyIsLabelledAnEstimate(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	var rsp ETAResponse
	// King's Cross to Heathrow, given as coordinates so nothing is geocoded.
	err := Server{}.ETA(context.Background(), &ETARequest{
		FromLat: 51.5308, FromLon: -0.1238,
		ToLat: 51.4700, ToLon: -0.4543,
		Mode: "drive",
	}, &rsp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Estimated") {
		t.Errorf("an estimate is not labelled as one: %q", rsp.Text)
	}
	// ~24km apart; the answer should be in that neighbourhood, not metres or
	// hundreds of kilometres.
	if !strings.Contains(rsp.Text, "km") {
		t.Errorf("no distance in the answer: %q", rsp.Text)
	}
}

// A journey needs both ends. Asking for one and getting a route from nowhere
// would be worse than being told what is missing.
func TestETANeedsBothEnds(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	var rsp ETAResponse
	if err := (Server{}).ETA(context.Background(), &ETARequest{ToLat: 51.47, ToLon: -0.45}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(rsp.Text), "starts") {
		t.Errorf("a missing origin was not reported: %q", rsp.Text)
	}

	rsp = ETAResponse{}
	if err := (Server{}).ETA(context.Background(), &ETARequest{FromLat: 51.53, FromLon: -0.12}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(rsp.Text), "ends") {
		t.Errorf("a missing destination was not reported: %q", rsp.Text)
	}
}

// Mode changes the answer — a walk is not a drive. Routing preference is also
// only valid for driving, which is why the mode is resolved rather than passed
// through.
func TestTravelModeIsResolved(t *testing.T) {
	for word, want := range map[string]string{
		"walk": "WALK", "walking": "WALK", "foot": "WALK",
		"cycle": "BICYCLE", "bike": "BICYCLE",
		"transit": "TRANSIT", "train": "TRANSIT",
		"drive": "DRIVE", "": "DRIVE",
	} {
		got, known := travelMode(word)
		if got != want || !known {
			t.Errorf("travelMode(%q) = %q, %v, want %q, true", word, got, known, want)
		}
	}
}

// TestAnUnknownModeIsRefusedRatherThanDriven. It used to answer DRIVE to
// anything it did not recognise, so "by helicopter" and "by ferry" came back as
// confident driving times with nothing to say the question had been changed.
func TestAnUnknownModeIsRefusedRatherThanDriven(t *testing.T) {
	for _, word := range []string{"helicopter", "ferry", "nonsense", "teleport"} {
		if _, known := travelMode(word); known {
			t.Errorf("travelMode(%q) claimed to know it", word)
		}
	}

	t.Setenv("GOOGLE_API_KEY", "")
	var rsp ETAResponse
	if err := (Server{}).ETA(context.Background(), &ETARequest{
		FromLat: 51.5308, FromLon: -0.1238, ToLat: 51.47, ToLon: -0.4543,
		Mode: "helicopter",
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "helicopter") {
		t.Errorf("the refusal does not say what was refused: %q", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "drive") || !strings.Contains(rsp.Text, "transit") {
		t.Errorf("the refusal does not say what would work: %q", rsp.Text)
	}
	if strings.Contains(rsp.Text, "km") {
		t.Errorf("a helicopter journey was answered with a driving distance: %q", rsp.Text)
	}
}

// TestATimeIsReadOrRefused — the two times are alternatives, not a pair.
func TestATimeIsReadOrRefused(t *testing.T) {
	if _, err := journeyTime("2026-08-13T08:00:00Z", "2026-08-13T09:00:00Z"); err == nil {
		t.Error("accepted both a departure and an arrival, which is two questions")
	}
	if _, err := journeyTime("tomorrow morning", ""); err == nil {
		t.Error("accepted something that is not a time")
	}
	w, err := journeyTime("2026-08-13T08:00:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Depart.IsZero() || !w.Arrive.IsZero() {
		t.Errorf("journeyTime = %+v", w)
	}
	if w, err = journeyTime("", "2026-08-13T15:00:00Z"); err != nil || w.Arrive.IsZero() {
		t.Errorf("journeyTime = %+v, %v", w, err)
	}
	// Neither is now, which is what every call used to mean.
	if w, err = journeyTime("", ""); err != nil || !w.Depart.IsZero() || !w.Arrive.IsZero() {
		t.Errorf("journeyTime = %+v, %v", w, err)
	}
}

// TestTheJourneyTimeReachesTheProvider. departureTime is the whole point of
// asking about Monday morning rather than now, and it is invisible from the
// answer — so the request itself is what has to be checked.
func TestTheJourneyTimeReachesTheProvider(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "a-key")

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck
		w.Write([]byte(`{"routes":[{"duration":"1800s","staticDuration":"1500s","distanceMeters":24000}]}`))
	}))
	defer srv.Close()
	orig := googleRoutesURL
	googleRoutesURL = srv.URL
	defer func() { googleRoutesURL = orig }()

	future := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)

	// Driving, leaving later: the provider is told when.
	if _, err := computeRoute(51.53, -0.12, 51.47, -0.45, "DRIVE", when{Depart: future}, summary); err != nil {
		t.Fatal(err)
	}
	if got["departureTime"] != future.Format(time.RFC3339) {
		t.Errorf("departureTime = %v, want %s", got["departureTime"], future.Format(time.RFC3339))
	}

	// Transit, arriving by: only transit can be asked to arrive by a time.
	got = nil
	if _, err := computeRoute(51.53, -0.12, 51.47, -0.45, "TRANSIT", when{Arrive: future}, summary); err != nil {
		t.Fatal(err)
	}
	if got["arrivalTime"] != future.Format(time.RFC3339) {
		t.Errorf("arrivalTime = %v", got["arrivalTime"])
	}
	if got["departureTime"] != nil {
		t.Error("sent both a departure and an arrival time")
	}

	// Driving, arriving by: asked about the road at that hour instead, since
	// only transit takes an arrivalTime.
	got = nil
	if _, err := computeRoute(51.53, -0.12, 51.47, -0.45, "DRIVE", when{Arrive: future}, summary); err != nil {
		t.Fatal(err)
	}
	if got["arrivalTime"] != nil {
		t.Error("sent an arrivalTime for a mode that does not take one")
	}
	if got["departureTime"] != future.Format(time.RFC3339) {
		t.Errorf("departureTime = %v", got["departureTime"])
	}

	// A time that has already been is dropped rather than sent and refused.
	got = nil
	past := time.Now().Add(-24 * time.Hour)
	if _, err := computeRoute(51.53, -0.12, 51.47, -0.45, "DRIVE", when{Depart: past}, summary); err != nil {
		t.Fatal(err)
	}
	if got["departureTime"] != nil {
		t.Error("sent a departure time in the past, which the provider refuses outright")
	}
}

// TestTrafficIsSeparatedFromTheJourney — "34 minutes" and "34 minutes, 11 of
// them traffic" are different answers to somebody deciding when to leave, and
// the second costs one more field on the same request.
func TestTrafficIsSeparatedFromTheJourney(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "a-key")

	var mask string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mask = r.Header.Get("X-Goog-FieldMask")
		w.Write([]byte(`{"routes":[{"duration":"2040s","staticDuration":"1380s","distanceMeters":24000}]}`))
	}))
	defer srv.Close()
	orig := googleRoutesURL
	googleRoutesURL = srv.URL
	defer func() { googleRoutesURL = orig }()

	r, err := computeRoute(51.53, -0.12, 51.47, -0.45, "DRIVE", when{}, summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mask, "staticDuration") {
		t.Errorf("the traffic-free duration was not asked for: %q", mask)
	}
	if r.Delay() != 11*time.Minute {
		t.Errorf("Delay = %s, want 11m (34 travelled, 23 on an empty road)", r.Delay())
	}

	// A mode with no traffic reports none rather than a negative delay.
	if (route{Duration: 20 * time.Minute}).Delay() != 0 {
		t.Error("a journey with no static duration invented a delay")
	}
	if (route{Duration: 20 * time.Minute, Static: 25 * time.Minute}).Delay() != 0 {
		t.Error("a journey faster than its empty road reported negative traffic")
	}
}

func TestEstimateScalesWithMode(t *testing.T) {
	drive := estimateRoute(51.5308, -0.1238, 51.4700, -0.4543, "DRIVE")
	walk := estimateRoute(51.5308, -0.1238, 51.4700, -0.4543, "WALK")
	if walk.Duration <= drive.Duration {
		t.Errorf("walking (%s) is not slower than driving (%s)", walk.Duration, drive.Duration)
	}
	if drive.Metres != walk.Metres {
		t.Error("the same journey changed length with the mode")
	}
	if !drive.Estimate {
		t.Error("a straight-line guess is not marked as an estimate")
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "under a minute",
		22 * time.Minute: "22 min",
		60 * time.Minute: "1 hr",
		65 * time.Minute: "1 hr 5 min",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestHumanDistance(t *testing.T) {
	if got := humanDistance(600); got != "600 m" {
		t.Errorf("short distances should be metres, got %q", got)
	}
	if got := humanDistance(24300); got != "24.3 km" {
		t.Errorf("long distances should be km, got %q", got)
	}
}

// A key that reaches Places does not necessarily have Routes enabled. Live
// testing found exactly that: a 403 saying "Requests to this API ... are
// blocked", returned verbatim to somebody who asked how long a journey takes.
// An estimate, labelled as one, is a better answer than an API error.
func TestRoutingFailureFallsBackToAnEstimate(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "a-key-without-routes-enabled")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":403,"message":"blocked"}}`))
	}))
	defer srv.Close()

	orig := googleRoutesURL
	googleRoutesURL = srv.URL
	defer func() { googleRoutesURL = orig }()

	r, err := computeRoute(51.5308, -0.1238, 51.47, -0.4543, "DRIVE", when{}, summary)
	if err != nil {
		t.Fatalf("a blocked routing API produced an error instead of an estimate: %v", err)
	}
	if !r.Estimate {
		t.Error("the fallback is not marked as an estimate")
	}
	if r.Duration <= 0 || r.Metres <= 0 {
		t.Errorf("the estimate is empty: %+v", r)
	}
}

// A genuinely impossible journey is an answer, not a fallback: estimating a
// drive to an island would be worse than saying there is no route.
func TestNoRouteIsReportedRatherThanEstimated(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "a-key")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"routes":[]}`))
	}))
	defer srv.Close()

	orig := googleRoutesURL
	googleRoutesURL = srv.URL
	defer func() { googleRoutesURL = orig }()

	if _, err := computeRoute(51.5, -0.1, 40.7, -74.0, "DRIVE", when{}, summary); err == nil {
		t.Error("an impossible journey was estimated rather than reported")
	}
}
