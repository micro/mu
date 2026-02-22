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

// TestSchemaVersionWipesOldData verifies that an existing places database that
// has no schema_version record (i.e. pre-versioning Overpass/Foursquare data)
// is wiped on init so that stale rows cannot block fresh Google Places results.
func TestSchemaVersionWipesOldData(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// ---- Phase 1: simulate an old (unversioned) database ----
	// Open the DB directly and populate it without a schema_version table.
	placesDBOne = sync.Once{}
	placesDB = nil

	// Boot the DB once to create tables.
	if err := initPlacesDB(); err != nil {
		t.Fatalf("initPlacesDB: %v", err)
	}

	// Remove the schema_version table and record to simulate old data.
	if _, err := placesDB.Exec(`DELETE FROM schema_version`); err != nil {
		t.Fatalf("delete schema_version: %v", err)
	}
	if _, err := placesDB.Exec(`DROP TABLE IF EXISTS schema_version`); err != nil {
		t.Fatalf("drop schema_version: %v", err)
	}

	// Insert a "legacy" place that should be wiped.
	if _, err := placesDB.Exec(
		`INSERT INTO places (id, name, category, lat, lon, geohash) VALUES ('legacy_1', 'Old Overpass Place', 'cafe', 1.0, 1.0, 'abcdef')`,
	); err != nil {
		t.Fatalf("insert legacy place: %v", err)
	}

	// Close the current connection so a fresh init can open it.
	placesDB.Close()
	placesDB = nil
	placesDBOne = sync.Once{}

	// ---- Phase 2: re-init should detect missing version and wipe ----
	if err := initPlacesDB(); err != nil {
		t.Fatalf("initPlacesDB after wipe: %v", err)
	}

	// The legacy place must be gone.
	var count int
	if err := placesDB.QueryRow(`SELECT COUNT(*) FROM places WHERE id = 'legacy_1'`).Scan(&count); err != nil {
		t.Fatalf("count legacy place: %v", err)
	}
	if count != 0 {
		t.Errorf("expected legacy place to be wiped, but found %d row(s)", count)
	}

	// The schema_version table must exist and contain schemaVersion.
	var ver string
	if err := placesDB.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&ver); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if ver != schemaVersion {
		t.Errorf("expected schema version %q, got %q", schemaVersion, ver)
	}
}

// TestSchemaVersionPreservesData verifies that a database that already has the
// correct schema version is NOT wiped on re-init.
func TestSchemaVersionPreservesData(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	placesDBOne = sync.Once{}
	placesDB = nil

	if err := initPlacesDB(); err != nil {
		t.Fatalf("initPlacesDB: %v", err)
	}

	// Insert a place that should survive a re-init.
	if _, err := placesDB.Exec(
		`INSERT INTO places (id, name, category, lat, lon, geohash) VALUES ('gpl:keep_me', 'Good Place', 'cafe', 1.0, 1.0, 'abcdef')`,
	); err != nil {
		t.Fatalf("insert place: %v", err)
	}

	// Re-init without resetting the DB handle — simulates a new connection on
	// a DB that already has the correct version.
	placesDB.Close()
	placesDB = nil
	placesDBOne = sync.Once{}

	if err := initPlacesDB(); err != nil {
		t.Fatalf("initPlacesDB second time: %v", err)
	}

	var count int
	if err := placesDB.QueryRow(`SELECT COUNT(*) FROM places WHERE id = 'gpl:keep_me'`).Scan(&count); err != nil {
		t.Fatalf("count place: %v", err)
	}
	if count != 1 {
		t.Errorf("expected place to be preserved, got count=%d", count)
	}
}
