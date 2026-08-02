package places

import (
	"strings"
	"testing"
)

func TestResolveLocation(t *testing.T) {
	// Explicit coordinates win and are returned as-is.
	if lat, lon, ok := resolveLocation("anything", 51.5, -0.12); !ok || lat != 51.5 || lon != -0.12 {
		t.Errorf("explicit coords: got %v,%v ok=%v", lat, lon, ok)
	}
	// No location at all → not resolvable.
	if _, _, ok := resolveLocation("", 0, 0); ok {
		t.Error("empty near + zero coords should not resolve")
	}
}

func TestLocationLabel(t *testing.T) {
	if got := locationLabel("Shoreditch", 1, 2); got != "Shoreditch" {
		t.Errorf("name should win: %q", got)
	}
	if got := locationLabel("", 51.5074, -0.1278); !strings.Contains(got, "51.50") {
		t.Errorf("coords fallback: %q", got)
	}
}

func TestFormatDistance(t *testing.T) {
	if got := formatDistance(450); got != "450m" {
		t.Errorf("450m: %q", got)
	}
	if got := formatDistance(2500); got != "2.5km" {
		t.Errorf("2.5km: %q", got)
	}
}

func TestRenderPlaces(t *testing.T) {
	if got := renderPlaces("test", nil, true); !strings.Contains(got, "No places") {
		t.Errorf("empty: %q", got)
	}
	places := []*Place{
		{Name: "Blue Bottle", Address: "1 Main St", Distance: 320, OpeningHours: "07:00-19:00", Phone: "555-1234"},
		{DisplayName: "Fallback Name", Distance: 1500},
	}
	got := renderPlaces("coffee", places, true)
	if !strings.Contains(got, "Blue Bottle") || !strings.Contains(got, "1 Main St") {
		t.Errorf("missing name/address: %q", got)
	}
	if !strings.Contains(got, "320m away") {
		t.Errorf("missing distance: %q", got)
	}
	if !strings.Contains(got, "07:00-19:00") || !strings.Contains(got, "555-1234") {
		t.Errorf("missing extras: %q", got)
	}
	if !strings.Contains(got, "Fallback Name") {
		t.Errorf("display-name fallback missing: %q", got)
	}
}

// TestRenderPlacesCap verifies the output is capped at 10 entries.
func TestRenderPlacesCap(t *testing.T) {
	var many []*Place
	for i := 0; i < 25; i++ {
		many = append(many, &Place{Name: "P"})
	}
	got := renderPlaces("many", many, false)
	if n := strings.Count(got, "\n- "); n > 10 {
		t.Errorf("expected <=10 entries, got %d", n)
	}
}
