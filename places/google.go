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

const googlePlacesBaseURL = "https://maps.googleapis.com/maps/api/place"

// googleAPIKey returns the Google Places API key from the environment.
func googleAPIKey() string {
	return os.Getenv("GOOGLE_API_KEY")
}

// googlePlacesResult represents a single place from the Google Places API.
type googlePlacesResult struct {
	PlaceID  string   `json:"place_id"`
	Name     string   `json:"name"`
	Types    []string `json:"types"`
	Vicinity string   `json:"vicinity"`
	Geometry struct {
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
	} `json:"geometry"`
	OpeningHours *struct {
		OpenNow bool `json:"open_now"`
	} `json:"opening_hours,omitempty"`
	Rating               float64 `json:"rating,omitempty"`
	UserRatingsTotal     int     `json:"user_ratings_total,omitempty"`
	FormattedPhoneNumber string  `json:"formatted_phone_number,omitempty"`
	Website              string  `json:"website,omitempty"`
}

type googlePlacesResponse struct {
	Results []googlePlacesResult `json:"results"`
	Status  string               `json:"status"`
}

// googleNearby fetches POIs near a location using the Google Places Nearby Search API.
// Returns nil, nil when GOOGLE_API_KEY is not set.
func googleNearby(lat, lon float64, radiusM int) ([]*Place, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, nil
	}
	apiURL := fmt.Sprintf(
		"%s/nearbysearch/json?location=%f,%f&radius=%d&key=%s",
		googlePlacesBaseURL, lat, lon, radiusM, url.QueryEscape(key),
	)
	return googleDo(apiURL)
}

// googleSearch searches for POIs near a location matching a keyword using the
// Google Places Text Search API. Returns nil, nil when GOOGLE_API_KEY is not set.
func googleSearch(query string, lat, lon float64, radiusM int) ([]*Place, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, nil
	}
	apiURL := fmt.Sprintf(
		"%s/textsearch/json?query=%s&location=%f,%f&radius=%d&key=%s",
		googlePlacesBaseURL, url.QueryEscape(query), lat, lon, radiusM, url.QueryEscape(key),
	)
	return googleDo(apiURL)
}

// googleDo executes a Google Places API GET request and returns parsed places.
func googleDo(apiURL string) ([]*Place, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google places request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google places returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var gResp googlePlacesResponse
	if err := json.Unmarshal(body, &gResp); err != nil {
		return nil, err
	}

	if gResp.Status != "OK" && gResp.Status != "ZERO_RESULTS" {
		return nil, fmt.Errorf("google places API error: %s", gResp.Status)
	}

	return parseGooglePlaces(gResp.Results), nil
}

// parseGooglePlaces converts Google Places API results into Place structs.
func parseGooglePlaces(results []googlePlacesResult) []*Place {
	places := make([]*Place, 0, len(results))
	for _, r := range results {
		if r.Name == "" {
			continue
		}
		lat := r.Geometry.Location.Lat
		lon := r.Geometry.Location.Lng
		if lat == 0 && lon == 0 {
			continue
		}

		category := ""
		if len(r.Types) > 0 {
			// Filter out generic Google types to get the most specific category
			for _, t := range r.Types {
				if t != "point_of_interest" && t != "establishment" {
					category = strings.ReplaceAll(t, "_", " ")
					break
				}
			}
		}

		places = append(places, &Place{
			ID:       "gpl:" + r.PlaceID,
			Name:     r.Name,
			Category: category,
			Address:  r.Vicinity,
			Lat:      lat,
			Lon:      lon,
			Phone:    r.FormattedPhoneNumber,
			Website:  r.Website,
		})
	}
	return places
}
