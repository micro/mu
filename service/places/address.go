package places

// The other direction: coordinates to an address.
//
// places_geocode turned a name into a point and there was nothing to turn a
// point back into a name — which is the half you need more often, because
// coordinates are what everything else here hands you. A phone's location, a
// photo's EXIF, the centre of a search, the end of a route: all of them are a
// pair of numbers that no reply can say out loud.
//
// Nominatim again, so it costs nothing and needs no key. Google has a Geocoding
// API that would do this too, and it is not worth a paid call to answer a
// question OpenStreetMap answers well.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// nominatimReverseURL is a var so a test can point it at a stub.
var nominatimReverseURL = "https://nominatim.openstreetmap.org/reverse"

// reverseGeocode names the place at a point.
//
// Zoom 18 is building level. Lower would name the street or the suburb, which
// sounds more forgiving and is worse: asked what is at a point, "London" is not
// wrong but it is not an answer either.
func reverseGeocode(lat, lon float64) (*nominatimResult, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, fmt.Errorf("%.4f, %.4f is not a point on Earth", lat, lon)
	}
	url := fmt.Sprintf("%s?lat=%f&lon=%f&format=json&zoom=18&addressdetails=1",
		nominatimReverseURL, lat, lon)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mu/1.0 (https://your-instance.com)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Nothing there is an object with an error field rather than an empty list,
	// because the reverse endpoint returns one result or none.
	var out nominatimResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.DisplayName) == "" {
		// The middle of an ocean is a legitimate question with no answer.
		return nil, fmt.Errorf("nothing is mapped at %.4f, %.4f", lat, lon)
	}
	return &out, nil
}
