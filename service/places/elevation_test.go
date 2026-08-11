package places

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestElevationCachesForever(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Write([]byte(`{"elevation":[1609.0]}`))
	}))
	defer srv.Close()
	old := elevationBaseURL
	elevationBaseURL, elevationCache = srv.URL, map[string]float64{}
	defer func() { elevationBaseURL = old }()

	for i := 0; i < 3; i++ {
		m, err := Elevation(39.7392, -104.9903)
		if err != nil {
			t.Fatal(err)
		}
		if m != 1609 {
			t.Fatalf("elevation = %v, want 1609", m)
		}
	}
	if calls != 1 {
		t.Errorf("asked the same hillside %d times, want 1 — the ground does not move", calls)
	}
}

func TestDescribeElevationGivesBothUnits(t *testing.T) {
	// Metres are the measurement; feet are what aviation, hiking maps and most
	// of the people asking still use.
	got := describeElevation("Denver", 39.7392, -104.9903, 1609)
	for _, want := range []string{"Denver", "1609 m", "5279 ft", "above sea level"} {
		if !strings.Contains(got, want) {
			t.Errorf("describeElevation() = %q, missing %q", got, want)
		}
	}

	// The Dead Sea is a real answer, not an error, and "-430 m above sea level"
	// is not how anyone says it.
	below := describeElevation("Dead Sea", 31.5, 35.5, -430)
	if !strings.Contains(below, "below sea level") || strings.Contains(below, "-430") {
		t.Errorf("a place below sea level read as %q", below)
	}
}
