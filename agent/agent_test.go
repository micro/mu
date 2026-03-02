package agent

import (
	"strings"
	"testing"
)

func TestPlacesMapURL_QueryAndNear(t *testing.T) {
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	items := []placeItem{{Name: "Test Cafe", Lat: 51.4, Lon: -0.37}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "cafe") {
		t.Errorf("expected query 'cafe' in URL, got %q", got)
	}
	if !strings.Contains(got, "Hampton") {
		t.Errorf("expected 'Hampton' in URL, got %q", got)
	}
}

func TestPlacesMapURL_QueryOnly(t *testing.T) {
	args := map[string]any{"q": "pharmacy"}
	items := []placeItem{{Name: "Boots", Lat: 51.5, Lon: -0.1}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "pharmacy") {
		t.Errorf("expected 'pharmacy' in URL, got %q", got)
	}
}

func TestPlacesMapURL_AddressArg(t *testing.T) {
	// places_nearby uses "address" instead of "near"; without a keyword
	// query the function should fall back to coordinate-based centering.
	args := map[string]any{"address": "London"}
	items := []placeItem{{Name: "Park", Lat: 51.5, Lon: -0.1}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	// Coordinate-based fallback should embed the place's lat/lon.
	if !strings.Contains(got, "51.5") {
		t.Errorf("expected latitude in coordinate fallback URL, got %q", got)
	}
}

func TestPlacesMapURL_FallbackToCoordinates(t *testing.T) {
	args := map[string]any{}
	items := []placeItem{{Name: "Mystery Place", Lat: 51.4, Lon: -0.37}}
	got := placesMapURL(args, items)
	if !strings.Contains(got, "google.com/maps") {
		t.Errorf("expected google maps URL, got %q", got)
	}
	if !strings.Contains(got, "51.4") {
		t.Errorf("expected latitude in URL, got %q", got)
	}
}

func TestPlacesMapURL_FallbackToPlacesPage(t *testing.T) {
	// No args, no coordinate data → /places
	got := placesMapURL(nil, []placeItem{{Name: "No Coords"}})
	if got != "/places" {
		t.Errorf("expected /places fallback, got %q", got)
	}
}

func TestFormatPlacesResult_WithResults(t *testing.T) {
	result := `{"results":[{"name":"Blue Cafe","category":"cafe","address":"12 High St"},{"name":"Red Cafe","category":"cafe","address":"5 Market St"}],"count":2}`
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	got := formatPlacesResult(result, args)
	if !strings.Contains(got, "Blue Cafe") {
		t.Errorf("expected 'Blue Cafe' in output, got %q", got)
	}
	if !strings.Contains(got, "Red Cafe") {
		t.Errorf("expected 'Red Cafe' in output, got %q", got)
	}
	if !strings.Contains(got, "Hampton") {
		t.Errorf("expected location in header, got %q", got)
	}
	if !strings.Contains(got, "cafe") {
		t.Errorf("expected query in header, got %q", got)
	}
}

func TestFormatPlacesResult_EmptyResults(t *testing.T) {
	result := `{"results":[],"count":0}`
	got := formatPlacesResult(result, nil)
	if got != "No places found." {
		t.Errorf("expected 'No places found.', got %q", got)
	}
}

func TestFormatPlacesResult_InvalidJSON(t *testing.T) {
	result := `not json`
	got := formatPlacesResult(result, nil)
	// Should fall back to original result
	if got != result {
		t.Errorf("expected original result as fallback, got %q", got)
	}
}

func TestRenderPlacesCard_MapLink(t *testing.T) {
	result := `{"results":[{"name":"Hampton Cafe","category":"cafe","address":"1 High St, Hampton"}],"count":1}`
	args := map[string]any{"q": "cafe", "near": "Hampton, UK"}
	card := renderPlacesCard(result, args)
	if !strings.Contains(card, "google.com/maps") {
		t.Errorf("expected google maps link in card, got %q", card)
	}
	if !strings.Contains(card, "Open in Google Maps ↗") {
		t.Errorf("expected 'Open in Google Maps ↗' link text, got %q", card)
	}
	if strings.Contains(card, `href="/places"`) {
		t.Errorf("card should not contain generic /places link, got %q", card)
	}
}

func TestRenderPlacesCard_Empty(t *testing.T) {
	got := renderPlacesCard(`{"results":[],"count":0}`, nil)
	if got != "" {
		t.Errorf("expected empty string for empty results, got %q", got)
	}
}
