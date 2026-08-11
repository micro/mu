package geo

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDistanceAgreesAcrossUnits(t *testing.T) {
	// London to New York. The same great circle, so the three answers must be
	// the same arc wearing different units — this is what caught the formula
	// being written out twice, once per unit.
	const lat1, lon1, lat2, lon2 = 51.5074, -0.1278, 40.7128, -74.0060

	km := DistanceKM(lat1, lon1, lat2, lon2)
	if km < 5500 || km > 5600 {
		t.Errorf("London to New York = %.0f km, want about 5570", km)
	}
	if got, want := DistanceM(lat1, lon1, lat2, lon2), km*1000; math.Abs(got-want) > 1 {
		t.Errorf("metres and kilometres disagree: %.0f vs %.0f", got, want)
	}
	if got, want := DistanceNM(lat1, lon1, lat2, lon2), km/1.852; math.Abs(got-want) > 1 {
		t.Errorf("nautical miles and kilometres disagree: %.1f vs %.1f", got, want)
	}
	if d := DistanceKM(lat1, lon1, lat1, lon1); d != 0 {
		t.Errorf("a point is %v from itself", d)
	}
}

func TestBearingPointsTheRightWay(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want                   float64
	}{
		{"due north", 0, 0, 10, 0, 0},
		{"due south", 10, 0, 0, 0, 180},
		{"due east", 0, 0, 0, 10, 90},
		{"due west", 0, 10, 0, 0, 270},
	}
	for _, c := range cases {
		got := Bearing(c.lat1, c.lon1, c.lat2, c.lon2)
		if math.Abs(got-c.want) > 0.5 {
			t.Errorf("%s: bearing %.1f, want %.1f", c.name, got, c.want)
		}
	}
}

func TestGeocodeCachesAndFailsHonestly(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("q") == "nowhere at all" {
			w.Write([]byte(`[]`))
			return
		}
		w.Write([]byte(`[{"lat":"51.5074","lon":"-0.1278"}]`))
	}))
	defer srv.Close()
	old := nominatimURL
	nominatimURL, cache = srv.URL, map[string]Point{}
	defer func() { nominatimURL = old }()

	for i := 0; i < 3; i++ {
		lat, lon, err := Geocode("London")
		if err != nil || math.Abs(lat-51.5074) > 0.001 || math.Abs(lon+0.1278) > 0.001 {
			t.Fatalf("Geocode(London) = %v, %v, %v", lat, lon, err)
		}
	}
	if calls != 1 {
		t.Errorf("asked about London %d times — towns do not move", calls)
	}

	// Nominatim is free and asks for restraint; an empty result must not be
	// cached as 0,0 and must not read as "the Atlantic".
	if _, _, err := Geocode("nowhere at all"); err == nil {
		t.Error("a place that does not exist geocoded successfully")
	}
	if _, _, err := Geocode(""); err == nil {
		t.Error("an empty query reached the provider")
	}
}
