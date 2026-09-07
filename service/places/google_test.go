package places

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type placesTransport func(*http.Request) (*http.Response, error)

func (f placesTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The provider must select nearby matches before limiting the response.
// Sorting a relevance-ranked shortlist locally cannot recover omitted cafes.
func TestGoogleSelectsByDistanceBeforeLimiting(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "test-key")
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	calls := 0
	httpClient = &http.Client{Transport: placesTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if r.URL.Path != "/v1/places:searchNearby" {
				t.Errorf("cafe query used relevance search: %s", r.URL.Path)
			}
			if types, ok := body["includedTypes"].([]interface{}); !ok || len(types) != 1 || types[0] != "cafe" {
				t.Errorf("missing cafe filter: %v", body["includedTypes"])
			}
		}
		if body["rankPreference"] != "DISTANCE" {
			t.Errorf("%s ranking = %v", r.URL.Path, body["rankPreference"])
		}
		if body["maxResultCount"] != float64(20) {
			t.Errorf("unexpected result limit: %v", body["maxResultCount"])
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"places":[]}`))}, nil
	})}
	if _, err := googleSearch("cafe", 51.5, -0.1, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := googleNearby(51.5, -0.1, 1000); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("made %d requests, want 2", calls)
	}
}

func TestChangingSortKeepsTheNearestCafe(t *testing.T) {
	places := []*Place{{Name: "A cafe", Distance: 600}, {Name: "Z cafe", Distance: 40}, {Name: "B cafe", Distance: 650}}
	sortPlaces(places, "distance")
	if places[0].Distance != 40 {
		t.Fatal("nearest cafe missing from first position")
	}
	sortPlaces(places, "name")
	if len(places) != 3 || places[2].Distance != 40 {
		t.Fatal("name sort changed the result set")
	}
	sortPlaces(places, "distance")
	if places[0].Distance != 40 {
		t.Fatal("distance sort did not restore nearest cafe")
	}
}
