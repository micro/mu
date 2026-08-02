package places

import (
	"context"
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
		"drive": "DRIVE", "": "DRIVE", "nonsense": "DRIVE",
	} {
		if got := travelMode(word); got != want {
			t.Errorf("travelMode(%q) = %q, want %q", word, got, want)
		}
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
