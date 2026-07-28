package islam

import (
	"math"
	"testing"
)

func TestQiblaBearing(t *testing.T) {
	cases := []struct {
		name     string
		lat, lon float64
		want     float64 // known qibla bearing, degrees from true north
		tol      float64
	}{
		// Widely published qibla directions for these cities.
		{"London", 51.5074, -0.1278, 118.9, 1.5},
		{"New York", 40.7128, -74.0060, 58.5, 1.5},
		{"Jakarta", -6.2088, 106.8456, 295.1, 1.5},
		{"Cairo", 30.0444, 31.2357, 136.1, 1.5},
	}
	for _, c := range cases {
		got := QiblaBearing(c.lat, c.lon)
		if math.Abs(got-c.want) > c.tol {
			t.Errorf("%s: bearing = %.1f°, want %.1f° (±%.1f)", c.name, got, c.want, c.tol)
		}
		if got < 0 || got >= 360 {
			t.Errorf("%s: bearing %.1f out of range [0,360)", c.name, got)
		}
	}
}

// From the Kaaba itself the distance is zero; from London it is ~4750km.
func TestDistanceToMecca(t *testing.T) {
	if d := DistanceToMeccaKm(kaabaLat, kaabaLon); d > 1 {
		t.Errorf("distance from the Kaaba = %.1fkm, want ~0", d)
	}
	if d := DistanceToMeccaKm(51.5074, -0.1278); math.Abs(d-4780) > 150 {
		t.Errorf("London distance = %.0fkm, want ~4780", d)
	}
}

func TestCompassPoint(t *testing.T) {
	cases := map[float64]string{0: "N", 45: "NE", 90: "E", 180: "S", 270: "W", 359: "N", 118.9: "ESE"}
	for bearing, want := range cases {
		if got := CompassPoint(bearing); got != want {
			t.Errorf("CompassPoint(%.1f) = %q, want %q", bearing, got, want)
		}
	}
}
