package places

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const foursquareBaseURL = "https://api.foursquare.com/v3/places"

// fsqResponse is the top-level Foursquare Places API v3 response.
type fsqResponse struct {
	Results []fsqVenue `json:"results"`
}

// fsqVenue represents a single place from the Foursquare Places API.
type fsqVenue struct {
	FsqID      string        `json:"fsq_id"`
	Name       string        `json:"name"`
	Categories []fsqCategory `json:"categories"`
	Location   fsqLocation   `json:"location"`
	Geocodes   fsqGeocodes   `json:"geocodes"`
	Tel        string        `json:"tel"`
	Website    string        `json:"website"`
	Hours      *fsqHours     `json:"hours,omitempty"`
	Distance   int           `json:"distance"` // metres from the query point
}

type fsqCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type fsqLocation struct {
	Address          string `json:"address"`
	Locality         string `json:"locality"`
	Postcode         string `json:"postcode"`
	Country          string `json:"country"`
	FormattedAddress string `json:"formatted_address"`
}

type fsqGeocodes struct {
	Main struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"main"`
}

type fsqHours struct {
	Display string `json:"display"`
}

// foursquareAPIKey returns the Foursquare API key from the environment.
func foursquareAPIKey() string {
	return os.Getenv("FOURSQUARE_API_KEY")
}

// foursquareDo executes a Foursquare Places API GET request and returns parsed places.
// The Foursquare v3 API uses the bare API key in the Authorization header without a scheme prefix.
func foursquareDo(apiURL, key string) ([]*Place, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("foursquare request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("foursquare returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var fsqResp fsqResponse
	if err := json.Unmarshal(body, &fsqResp); err != nil {
		return nil, err
	}

	return parseFoursquareVenues(fsqResp.Results), nil
}

// foursquareNearby fetches POIs near a location using the Foursquare Places API.
// Returns nil, nil when FOURSQUARE_API_KEY is not set.
func foursquareNearby(lat, lon float64, radiusM int) ([]*Place, error) {
	key := foursquareAPIKey()
	if key == "" {
		return nil, nil
	}
	apiURL := fmt.Sprintf(
		"%s/nearby?ll=%f,%f&radius=%d&limit=50&fields=fsq_id,name,categories,location,geocodes,tel,website,hours,distance",
		foursquareBaseURL, lat, lon, radiusM,
	)
	return foursquareDo(apiURL, key)
}

// foursquareSearch searches for POIs near a location matching a keyword using the
// Foursquare Places API. Returns nil, nil when FOURSQUARE_API_KEY is not set.
func foursquareSearch(query string, lat, lon float64, radiusM int) ([]*Place, error) {
	key := foursquareAPIKey()
	if key == "" {
		return nil, nil
	}
	apiURL := fmt.Sprintf(
		"%s/search?query=%s&ll=%f,%f&radius=%d&limit=50&fields=fsq_id,name,categories,location,geocodes,tel,website,hours,distance",
		foursquareBaseURL, url.QueryEscape(query), lat, lon, radiusM,
	)
	return foursquareDo(apiURL, key)
}

// parseFoursquareVenues converts Foursquare API results into Place structs.
func parseFoursquareVenues(venues []fsqVenue) []*Place {
	places := make([]*Place, 0, len(venues))
	for _, v := range venues {
		if v.Name == "" {
			continue
		}
		lat := v.Geocodes.Main.Latitude
		lon := v.Geocodes.Main.Longitude
		// Zero geocodes mean the API did not return coordinates for this venue.
		// Real places at exactly (0°, 0°) in the ocean are never returned by the
		// Foursquare Places API for terrestrial searches, so this is safe.
		if lat == 0 && lon == 0 {
			continue
		}

		category := ""
		if len(v.Categories) > 0 {
			category = strings.ToLower(v.Categories[0].Name)
		}

		addr := v.Location.FormattedAddress
		if addr == "" {
			parts := []string{}
			if v.Location.Address != "" {
				parts = append(parts, v.Location.Address)
			}
			if v.Location.Locality != "" {
				parts = append(parts, v.Location.Locality)
			}
			if v.Location.Postcode != "" {
				parts = append(parts, v.Location.Postcode)
			}
			addr = strings.Join(parts, ", ")
		}

		hours := ""
		if v.Hours != nil {
			hours = v.Hours.Display
		}

		places = append(places, &Place{
			ID:           "fsq:" + v.FsqID,
			Name:         v.Name,
			Category:     category,
			Address:      addr,
			Lat:          lat,
			Lon:          lon,
			Phone:        v.Tel,
			Website:      v.Website,
			OpeningHours: hours,
			Distance:     float64(v.Distance),
		})
	}
	return places
}
