package places

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const googlePlacesBaseURL = "https://places.googleapis.com/v1/places"

// googleFieldMask lists the fields requested from the Places API (New).
const googleFieldMask = "places.id,places.displayName,places.types,places.primaryType,places.formattedAddress,places.location,places.regularOpeningHours,places.nationalPhoneNumber,places.websiteUri"

// googleMaxResults is the maximum number of results to request from the Places API (New).
const googleMaxResults = 20

// googleAPIKey returns the Google Places API key from the environment.
func googleAPIKey() string {
	return os.Getenv("GOOGLE_API_KEY")
}

// googlePlaceResult represents a single place from the Places API (New).
type googlePlaceResult struct {
	ID               string   `json:"id"`
	Types            []string `json:"types"`
	PrimaryType      string   `json:"primaryType"`
	FormattedAddress string   `json:"formattedAddress"`
	Location         struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	DisplayName *struct {
		Text string `json:"text"`
	} `json:"displayName"`
	RegularOpeningHours *struct {
		OpenNow bool `json:"openNow"`
	} `json:"regularOpeningHours,omitempty"`
	NationalPhoneNumber string `json:"nationalPhoneNumber,omitempty"`
	WebsiteUri          string `json:"websiteUri,omitempty"`
}

type googlePlacesResponse struct {
	Places []googlePlaceResult `json:"places"`
}

// googleNearby fetches POIs near a location using the Places API (New) Nearby Search.
// Returns nil, nil when GOOGLE_API_KEY is not set.
func googleNearby(lat, lon float64, radiusM int) ([]*Place, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, nil
	}
	body := map[string]interface{}{
		"maxResultCount": googleMaxResults,
		"locationRestriction": map[string]interface{}{
			"circle": map[string]interface{}{
				"center": map[string]interface{}{
					"latitude":  lat,
					"longitude": lon,
				},
				"radius": float64(radiusM),
			},
		},
	}
	return googleDo(googlePlacesBaseURL+":searchNearby", key, body)
}

// googleSearch searches for POIs near a location matching a keyword using the
// Places API (New) Text Search. Returns nil, nil when GOOGLE_API_KEY is not set.
func googleSearch(query string, lat, lon float64, radiusM int) ([]*Place, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, nil
	}
	body := map[string]interface{}{
		"textQuery":      query,
		"maxResultCount": googleMaxResults,
		"locationBias": map[string]interface{}{
			"circle": map[string]interface{}{
				"center": map[string]interface{}{
					"latitude":  lat,
					"longitude": lon,
				},
				"radius": float64(radiusM),
			},
		},
	}
	return googleDo(googlePlacesBaseURL+":searchText", key, body)
}

// googleDo executes a Places API (New) POST request and returns parsed places.
func googleDo(apiURL, key string, payload interface{}) ([]*Place, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	req.Header.Set("X-Goog-FieldMask", googleFieldMask)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google places request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google places returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var gResp googlePlacesResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, err
	}

	return parseGooglePlaces(gResp.Places), nil
}

// parseGooglePlaces converts Places API (New) results into Place structs.
func parseGooglePlaces(results []googlePlaceResult) []*Place {
	places := make([]*Place, 0, len(results))
	for _, r := range results {
		if r.DisplayName == nil || r.DisplayName.Text == "" {
			continue
		}
		lat := r.Location.Latitude
		lon := r.Location.Longitude
		if lat == 0 && lon == 0 {
			continue
		}

		category := r.PrimaryType
		if category == "" && len(r.Types) > 0 {
			// Filter out generic types to get the most specific category
			for _, t := range r.Types {
				if t != "point_of_interest" && t != "establishment" {
					category = t
					break
				}
			}
		}
		category = strings.ReplaceAll(category, "_", " ")

		places = append(places, &Place{
			ID:       "gpl:" + r.ID,
			Name:     r.DisplayName.Text,
			Category: category,
			Address:  r.FormattedAddress,
			Lat:      lat,
			Lon:      lon,
			Phone:    r.NationalPhoneNumber,
			Website:  r.WebsiteUri,
		})
	}
	return places
}
