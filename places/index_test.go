package places

import (
	"os"
	"sync"
	"testing"
)

func TestEncodeGeohash(t *testing.T) {
	tests := []struct {
		lat, lon  float64
		precision int
		want      string
	}{
		// London approx geohash at precision 4
		{51.5074, -0.1278, 4, "gcpv"},
		// New York approx geohash at precision 4
		{40.7128, -74.006, 4, "dr5r"},
	}
	for _, tt := range tests {
		got := encodeGeohash(tt.lat, tt.lon, tt.precision)
		if got != tt.want {
			t.Errorf("encodeGeohash(%.4f, %.4f, %d) = %q, want %q",
				tt.lat, tt.lon, tt.precision, got, tt.want)
		}
	}
}

func TestEncodeGeohashLength(t *testing.T) {
	for _, prec := range []int{1, 3, 6, 9} {
		gh := encodeGeohash(0, 0, prec)
		if len(gh) != prec {
			t.Errorf("expected geohash length %d, got %d (%s)", prec, len(gh), gh)
		}
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	tests := []struct {
		input string
		valid bool // whether result should be non-empty
	}{
		{"cafe", true},
		{"", false},
		{"   ", false},
		{`"dangerous"`, true},
		{"co*fee shop", true},
	}
	for _, tt := range tests {
		got := sanitizeFTSQuery(tt.input)
		if tt.valid && got == "" {
			t.Errorf("sanitizeFTSQuery(%q) returned empty, expected non-empty", tt.input)
		}
		if !tt.valid && got != "" {
			t.Errorf("sanitizeFTSQuery(%q) = %q, expected empty", tt.input, got)
		}
	}
}

func TestIndexAndSearchPlaces(t *testing.T) {
	// Use a temp directory for the test database
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Reset the once so a fresh DB is created under tmpDir
	placesDBOne = sync.Once{}
	placesDB = nil

	places := []*Place{
		{ID: "1", Name: "Blue Bottle Coffee", Category: "cafe", Address: "Market St", Lat: 37.7749, Lon: -122.4194},
		{ID: "2", Name: "Tartine Bakery", Category: "bakery", Address: "Guerrero St", Lat: 37.7611, Lon: -122.4243},
		{ID: "3", Name: "Zuni Cafe", Category: "restaurant", Address: "Market St", Lat: 37.7751, Lon: -122.4198},
	}

	indexPlaces(places)

	// Text-only search
	results, err := searchPlacesFTS("coffee", 0, 0, 0, false)
	if err != nil {
		t.Fatalf("searchPlacesFTS error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result for 'coffee', got 0")
	}
	if results[0].Name != "Blue Bottle Coffee" {
		t.Errorf("expected 'Blue Bottle Coffee', got %q", results[0].Name)
	}

	// Geo-only search (near Market St, SF)
	geoResults, err := searchPlacesFTS("", 37.7749, -122.4194, 500, true)
	if err != nil {
		t.Fatalf("searchPlacesFTS geo error: %v", err)
	}
	if len(geoResults) == 0 {
		t.Error("expected geo results, got 0")
	}

	// Text + geo search
	combined, err := searchPlacesFTS("cafe", 37.7749, -122.4194, 2000, true)
	if err != nil {
		t.Fatalf("searchPlacesFTS combined error: %v", err)
	}
	if len(combined) == 0 {
		t.Error("expected combined results, got 0")
	}

	// Results should be sorted by distance when hasRef=true
	for i := 1; i < len(geoResults); i++ {
		if geoResults[i].Distance < geoResults[i-1].Distance {
			t.Errorf("results not sorted by distance at index %d", i)
		}
	}
}

func TestIndexPlacesUpdatesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	placesDBOne = sync.Once{}
	placesDB = nil

	original := []*Place{
		{ID: "10", Name: "Old Name", Category: "shop", Lat: 1.0, Lon: 1.0},
	}
	indexPlaces(original)

	updated := []*Place{
		{ID: "10", Name: "New Name", Category: "shop", Lat: 1.0, Lon: 1.0},
	}
	indexPlaces(updated)

	results, err := searchPlacesFTS("New Name", 0, 0, 0, false)
	if err != nil {
		t.Fatalf("searchPlacesFTS error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected updated place to be findable by new name")
	}
}
