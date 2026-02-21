package places

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/asim/quadtree"

	"mu/app"
	"mu/data"
)

//go:embed locations.json
var locationsJSON []byte

// CityDef defines a known city for pre-loading places data
type CityDef struct {
	Name     string  `json:"name"`
	Country  string  `json:"country"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	RadiusKm float64 `json:"radius_km"`
}

var cities []CityDef
var qtree *quadtree.QuadTree

const (
	maxPlacesPerCity = 2000
	cityFetchTimeout = 60 * time.Second
	// minLocalResults is the minimum number of quadtree results required before
	// we skip the Overpass fallback in findNearbyPlaces.
	minLocalResults = 5
)

// Cities returns the loaded city definitions.
func Cities() []CityDef {
	return cities
}

// cacheFileKey returns the data-store key for a city's cached places.
func cacheFileKey(cityName string) string {
	key := strings.ToLower(cityName)
	key = strings.ReplaceAll(key, " ", "_")
	return "places/" + key + ".json"
}

// initCities parses the embedded city list and creates the global quadtree.
func initCities() {
	if err := json.Unmarshal(locationsJSON, &cities); err != nil {
		app.Log("places", "Failed to parse locations.json: %v", err)
		return
	}
	// Global quadtree: covers the whole world (lat ±90, lon ±180)
	center := quadtree.NewPoint(0, 0, nil)
	half := quadtree.NewPoint(90, 180, nil)
	boundary := quadtree.NewAABB(center, half)

	mutex.Lock()
	qtree = quadtree.New(boundary, 0, nil)
	mutex.Unlock()
}

// loadCityCaches reads each city's JSON cache from disk and inserts into qtree.
// Returns the number of cities that had cached data.
func loadCityCaches() int {
	loaded := 0
	for _, city := range cities {
		var places []*Place
		if err := data.LoadJSON(cacheFileKey(city.Name), &places); err != nil || len(places) == 0 {
			continue
		}
		mutex.Lock()
		for _, p := range places {
			qtree.Insert(quadtree.NewPoint(p.Lat, p.Lon, p))
		}
		mutex.Unlock()
		go indexPlaces(places)
		loaded++
	}
	return loaded
}

// fetchMissingCities is a background goroutine that fetches Overpass data for
// any city that has no cached data on disk.
func fetchMissingCities() {
	for _, city := range cities {
		// Skip if we already have cached data
		if b, err := data.LoadFile(cacheFileKey(city.Name)); err == nil && len(b) > 10 {
			continue
		}

		app.Log("places", "Fetching places for %s (%.0fkm radius)", city.Name, city.RadiusKm)
		radiusM := int(city.RadiusKm * 1000)

		places, err := fetchCityFromOverpass(city.Lat, city.Lon, radiusM)
		if err != nil {
			app.Log("places", "Failed to fetch %s: %v", city.Name, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Persist to disk cache
		if err := data.SaveJSON(cacheFileKey(city.Name), places); err != nil {
			app.Log("places", "Failed to save cache for %s: %v", city.Name, err)
		}

		// Insert into quadtree
		mutex.Lock()
		for _, p := range places {
			qtree.Insert(quadtree.NewPoint(p.Lat, p.Lon, p))
		}
		mutex.Unlock()
		go indexPlaces(places)

		app.Log("places", "Cached %d places for %s", len(places), city.Name)
		time.Sleep(3 * time.Second) // respect Overpass rate limits
	}
	app.Log("places", "City pre-loading complete")
}

// fetchCityFromOverpass fetches major named POIs for a city from the Overpass API.
// The query is intentionally focused on significant places to avoid huge payloads.
func fetchCityFromOverpass(lat, lon float64, radiusM int) ([]*Place, error) {
	client := &http.Client{Timeout: cityFetchTimeout}

	// Focused on significant, named POIs to keep response size manageable
	query := fmt.Sprintf(`[out:json][timeout:55];
(
  node["amenity"~"restaurant|cafe|bar|pub|hospital|school|university|museum|theatre|cinema|library|bank|pharmacy|hotel|place_of_worship|police|fire_station|post_office"]["name"](around:%d,%f,%f);
  way["amenity"~"restaurant|cafe|bar|pub|hospital|school|university|museum|theatre|cinema|library|bank|pharmacy|hotel|place_of_worship|police|fire_station|post_office"]["name"](around:%d,%f,%f);
  node["tourism"~"attraction|museum|hotel|viewpoint|theme_park|zoo|aquarium|gallery"]["name"](around:%d,%f,%f);
  way["tourism"~"attraction|museum|hotel|viewpoint|theme_park|zoo|aquarium|gallery"]["name"](around:%d,%f,%f);
  node["shop"]["name"](around:%d,%f,%f);
  way["shop"]["name"](around:%d,%f,%f);
  node["historic"]["name"](around:%d,%f,%f);
  way["historic"]["name"](around:%d,%f,%f);
);
out center;`, radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon,
		radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon)

	req, err := http.NewRequest("POST", "https://overpass-api.de/api/interpreter",
		strings.NewReader("data="+url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mu/1.0 (https://mu.xyz)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("overpass city fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ovResp overpassResponse
	if err := json.Unmarshal(body, &ovResp); err != nil {
		return nil, err
	}

	places := make([]*Place, 0, min(len(ovResp.Elements), maxPlacesPerCity))
	for _, el := range ovResp.Elements {
		name := el.Tags["name"]
		if name == "" {
			continue
		}

		// Resolve coordinates: nodes have lat/lon directly; ways expose a center
		elLat, elLon := el.Lat, el.Lon
		if el.Center != nil && elLat == 0 && elLon == 0 {
			elLat, elLon = el.Center.Lat, el.Center.Lon
		}
		if elLat == 0 && elLon == 0 {
			continue
		}

		category := el.Tags["amenity"]
		if category == "" {
			category = el.Tags["tourism"]
		}
		if category == "" {
			category = el.Tags["shop"]
		}
		if category == "" {
			category = el.Tags["historic"]
		}

		addr := el.Tags["addr:street"]
		if n := el.Tags["addr:housenumber"]; n != "" && addr != "" {
			addr = n + " " + addr
		} else if n != "" {
			addr = n
		}
		if c := el.Tags["addr:city"]; c != "" {
			if addr != "" {
				addr += ", " + c
			} else {
				addr = c
			}
		}
		if p := el.Tags["addr:postcode"]; p != "" {
			addr += " " + p
		}

		phone := el.Tags["phone"]
		if phone == "" {
			phone = el.Tags["contact:phone"]
		}
		website := el.Tags["website"]
		if website == "" {
			website = el.Tags["contact:website"]
		}
		cuisine := strings.ReplaceAll(el.Tags["cuisine"], ";", ", ")
		cuisine = strings.ReplaceAll(cuisine, "_", " ")

		places = append(places, &Place{
			ID:           fmt.Sprintf("%d", el.ID),
			Name:         name,
			Category:     category,
			Address:      strings.TrimSpace(addr),
			Lat:          elLat,
			Lon:          elLon,
			Phone:        phone,
			Website:      website,
			OpeningHours: el.Tags["opening_hours"],
			Cuisine:      cuisine,
		})

		if len(places) >= maxPlacesPerCity {
			break
		}
	}
	return places, nil
}

// queryLocalByKeyword queries the in-memory quadtree for places within radiusM
// metres whose name, category, or cuisine contains the query string (case-insensitive).
func queryLocalByKeyword(query string, lat, lon float64, radiusM int) []*Place {
	places := queryLocal(lat, lon, radiusM)
	if len(places) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	var filtered []*Place
	for _, p := range places {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Category), q) ||
			strings.Contains(strings.ToLower(p.Cuisine), q) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
// Returns nil if the quadtree is not yet initialised.
func queryLocal(lat, lon float64, radiusM int) []*Place {
	mutex.RLock()
	defer mutex.RUnlock()

	if qtree == nil {
		return nil
	}

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(float64(radiusM))
	boundary := quadtree.NewAABB(center, half)

	points := qtree.Search(boundary)

	results := make([]*Place, 0, len(points))
	for _, pt := range points {
		if p, ok := pt.Data().(*Place); ok {
			dist := haversine(lat, lon, p.Lat, p.Lon)
			if dist > float64(radiusM) {
				continue // bounding box is approximate; filter to actual radius
			}
			pCopy := *p
			pCopy.Distance = dist
			results = append(results, &pCopy)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})
	return results
}
