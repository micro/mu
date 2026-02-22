package places

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

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
	// minLocalResults is the minimum number of local results (from the quadtree
	// or SQLite FTS index) required before falling back to Google Places.
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
