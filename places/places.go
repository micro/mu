package places

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/wallet"
)

var mutex sync.RWMutex

// Place represents a geographic place
type Place struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Type        string  `json:"type"`
	Address     string  `json:"address"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DisplayName string  `json:"display_name"`
	Distance    float64 `json:"distance,omitempty"` // metres, set when sorting by proximity
}

// cache for search results (query -> places)
var searchCache = map[string][]*Place{}
var searchCacheTime = map[string]time.Time{}

const cacheTTL = 1 * time.Hour

// nominatimResult represents a result from the Nominatim API
type nominatimResult struct {
	PlaceID     int64  `json:"place_id"`
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Type        string `json:"type"`
	Class       string `json:"class"`
	Address     struct {
		Road        string `json:"road"`
		City        string `json:"city"`
		Town        string `json:"town"`
		Village     string `json:"village"`
		Country     string `json:"country"`
		Postcode    string `json:"postcode"`
		HouseNumber string `json:"house_number"`
	} `json:"address"`
}

// overpassElement represents a POI from the Overpass API
type overpassElement struct {
	ID   int64             `json:"id"`
	Lat  float64           `json:"lat"`
	Lon  float64           `json:"lon"`
	Tags map[string]string `json:"tags"`
}

type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

// httpClient is the shared HTTP client with timeout
var httpClient = &http.Client{Timeout: 15 * time.Second}

// Load initialises the places package
func Load() {
	initCities()
	loaded := loadCityCaches()
	app.Log("places", "Places loaded: %d/%d cities in quadtree", loaded, len(cities))
	go fetchMissingCities()
}

// searchNominatim searches for places using the Nominatim API
func searchNominatim(query string) ([]*Place, error) {
	// Check cache
	mutex.RLock()
	if places, ok := searchCache[query]; ok {
		if time.Since(searchCacheTime[query]) < cacheTTL {
			mutex.RUnlock()
			return places, nil
		}
	}
	mutex.RUnlock()

	apiURL := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=20&addressdetails=1",
		url.QueryEscape(query),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mu/1.0 (https://mu.xyz)")

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

	var results []nominatimResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, err
	}

	places := make([]*Place, 0, len(results))
	for _, r := range results {
		lat, err := strconv.ParseFloat(r.Lat, 64)
		if err != nil {
			continue
		}
		lon, err := strconv.ParseFloat(r.Lon, 64)
		if err != nil {
			continue
		}

		addr := buildAddress(r)
		name := extractDisplayName(r)

		places = append(places, &Place{
			ID:          fmt.Sprintf("%d", r.PlaceID),
			Name:        name,
			Category:    r.Class,
			Type:        r.Type,
			Address:     addr,
			Lat:         lat,
			Lon:         lon,
			DisplayName: r.DisplayName,
		})
	}

	// Store in cache
	mutex.Lock()
	searchCache[query] = places
	searchCacheTime[query] = time.Now()
	mutex.Unlock()

	return places, nil
}

// nearbyOverpass finds POIs near a location using Overpass API
func nearbyOverpass(lat, lon float64, radiusM int) ([]*Place, error) {
	cacheKey := fmt.Sprintf("nearby:%.4f:%.4f:%d", lat, lon, radiusM)

	// Check cache
	mutex.RLock()
	if places, ok := searchCache[cacheKey]; ok {
		if time.Since(searchCacheTime[cacheKey]) < cacheTTL {
			mutex.RUnlock()
			return places, nil
		}
	}
	mutex.RUnlock()

	// Overpass query for common POI types within radius
	query := fmt.Sprintf(`[out:json][timeout:25];
(
  node["amenity"](around:%d,%f,%f);
  node["shop"](around:%d,%f,%f);
  node["tourism"](around:%d,%f,%f);
  node["leisure"](around:%d,%f,%f);
);
out body;`, radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon, radiusM, lat, lon)

	apiURL := "https://overpass-api.de/api/interpreter"
	req, err := http.NewRequest("POST", apiURL, strings.NewReader("data="+url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mu/1.0 (https://mu.xyz)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("overpass request failed: %w", err)
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

	places := make([]*Place, 0, len(ovResp.Elements))
	for _, el := range ovResp.Elements {
		name := el.Tags["name"]
		if name == "" {
			continue // Skip unnamed POIs
		}

		category := el.Tags["amenity"]
		if category == "" {
			category = el.Tags["shop"]
		}
		if category == "" {
			category = el.Tags["tourism"]
		}
		if category == "" {
			category = el.Tags["leisure"]
		}

		addr := el.Tags["addr:street"]
		if n := el.Tags["addr:housenumber"]; n != "" {
			addr = n + " " + addr
		}
		if p := el.Tags["addr:postcode"]; p != "" {
			addr += " " + p
		}

		places = append(places, &Place{
			ID:       fmt.Sprintf("%d", el.ID),
			Name:     name,
			Category: category,
			Address:  strings.TrimSpace(addr),
			Lat:      el.Lat,
			Lon:      el.Lon,
		})
	}

	// Store in cache
	mutex.Lock()
	searchCache[cacheKey] = places
	searchCacheTime[cacheKey] = time.Now()
	mutex.Unlock()

	return places, nil
}

// findNearbyPlaces finds POIs near a location.
// It first queries the in-memory quadtree; if insufficient results are found
// there it falls back to the live Overpass API.
func findNearbyPlaces(lat, lon float64, radiusM int) ([]*Place, error) {
	local := queryLocal(lat, lon, radiusM)
	if len(local) >= minLocalResults {
		return local, nil
	}
	return nearbyOverpass(lat, lon, radiusM)
}

// geocode resolves an address/postcode to lat/lon using Nominatim
func geocode(address string) (float64, float64, error) {
	results, err := searchNominatim(address)
	if err != nil || len(results) == 0 {
		return 0, 0, fmt.Errorf("could not geocode address: %s", address)
	}
	return results[0].Lat, results[0].Lon, nil
}

// haversine returns the great-circle distance in metres between two lat/lon points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in metres
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// buildAddress constructs a short address string from a nominatim result
func buildAddress(r nominatimResult) string {
	parts := []string{}
	if r.Address.HouseNumber != "" && r.Address.Road != "" {
		parts = append(parts, r.Address.HouseNumber+" "+r.Address.Road)
	} else if r.Address.Road != "" {
		parts = append(parts, r.Address.Road)
	}
	city := r.Address.City
	if city == "" {
		city = r.Address.Town
	}
	if city == "" {
		city = r.Address.Village
	}
	if city != "" {
		parts = append(parts, city)
	}
	if r.Address.Postcode != "" {
		parts = append(parts, r.Address.Postcode)
	}
	if r.Address.Country != "" {
		parts = append(parts, r.Address.Country)
	}
	return strings.Join(parts, ", ")
}

// extractDisplayName gets a short name from a nominatim result
func extractDisplayName(r nominatimResult) string {
	parts := strings.SplitN(r.DisplayName, ",", 2)
	if len(parts) > 0 && parts[0] != "" {
		return strings.TrimSpace(parts[0])
	}
	return r.DisplayName
}

// Handler handles /places requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// Handle sub-routes: /places/search and /places/nearby
	switch r.URL.Path {
	case "/places/search":
		handleSearch(w, r)
		return
	case "/places/nearby":
		handleNearby(w, r)
		return
	}

	// Handle JSON API requests for /places
	if app.WantsJSON(r) {
		q := r.URL.Query().Get("q")
		if q != "" {
			results, err := searchNominatim(q)
			if err != nil {
				app.RespondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			app.RespondJSON(w, map[string]interface{}{"results": results})
			return
		}
		app.RespondJSON(w, map[string]interface{}{"results": []*Place{}})
		return
	}

	// Default: show the places page with map
	body := renderPlacesPage(r)
	app.Respond(w, r, app.Response{
		Title:       "Places",
		Description: "Search and discover places on a map",
		HTML:        body,
	})
}

// handleSearch handles place search requests (POST /places/search)
func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}

	// Require auth for search (charged operation)
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			app.Unauthorized(w, r)
		} else {
			app.RedirectToLogin(w, r)
		}
		return
	}

	// Check quota
	canProceed, useFree, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpPlacesSearch)
	if !canProceed {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusPaymentRequired, "Insufficient credits. Top up your wallet to continue.")
		} else {
			app.Respond(w, r, app.Response{
				Title: "Places",
				HTML:  `<p class="text-error">Insufficient credits. <a href="/wallet/topup">Top up your wallet</a> to search places.</p>` + renderPlacesPage(r),
			})
		}
		return
	}

	r.ParseForm()
	query := strings.TrimSpace(r.Form.Get("q"))
	if query == "" {
		app.BadRequest(w, r, "Search query required")
		return
	}

	// Optional location for proximity sorting
	var nearLat, nearLon float64
	hasNearLoc := false
	nearAddr := strings.TrimSpace(r.Form.Get("near"))
	nearLatStr := r.Form.Get("near_lat")
	nearLonStr := r.Form.Get("near_lon")
	if nearLatStr != "" && nearLonStr != "" {
		parsedLat, latErr := strconv.ParseFloat(nearLatStr, 64)
		parsedLon, lonErr := strconv.ParseFloat(nearLonStr, 64)
		if latErr == nil && lonErr == nil {
			nearLat, nearLon, hasNearLoc = parsedLat, parsedLon, true
		} else {
			app.Log("places", "Invalid near_lat/near_lon: %v %v", latErr, lonErr)
		}
	} else if nearAddr != "" {
		if glat, glon, gerr := geocode(nearAddr); gerr == nil {
			nearLat, nearLon, hasNearLoc = glat, glon, true
		} else {
			app.Log("places", "Geocode of near=%q failed: %v", nearAddr, gerr)
		}
	}

	// Perform search
	results, err := searchNominatim(query)
	if err != nil {
		app.Log("places", "Search error: %v", err)
		app.ServerError(w, r, "Search failed. Please try again.")
		return
	}

	// Sort by distance when a reference location is available
	if hasNearLoc {
		for i := range results {
			results[i].Distance = haversine(nearLat, nearLon, results[i].Lat, results[i].Lon)
		}
		sort.Slice(results, func(i, j int) bool {
			return results[i].Distance < results[j].Distance
		})
	}

	// Consume quota after successful operation
	if useFree {
		wallet.UseFreeSearch(acc.ID)
	} else if cost > 0 {
		wallet.DeductCredits(acc.ID, cost, wallet.OpPlacesSearch, map[string]interface{}{"query": query})
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
		return
	}

	// Render results page
	html := renderSearchResults(query, results, nearLat, nearLon)
	app.Respond(w, r, app.Response{
		Title:       "Places - " + query,
		Description: fmt.Sprintf("Search results for %s", query),
		HTML:        html,
	})
}

// handleNearby handles nearby place requests (POST /places/nearby)
func handleNearby(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}

	// Require auth for nearby search (charged operation)
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			app.Unauthorized(w, r)
		} else {
			app.RedirectToLogin(w, r)
		}
		return
	}

	// Check quota
	canProceed, useFree, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpPlacesNearby)
	if !canProceed {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusPaymentRequired, "Insufficient credits. Top up your wallet to continue.")
		} else {
			app.Respond(w, r, app.Response{
				Title: "Places",
				HTML:  `<p class="text-error">Insufficient credits. <a href="/wallet/topup">Top up your wallet</a> to search nearby places.</p>` + renderPlacesPage(r),
			})
		}
		return
	}

	r.ParseForm()

	var lat, lon float64
	address := strings.TrimSpace(r.Form.Get("address"))
	latStr := r.Form.Get("lat")
	lonStr := r.Form.Get("lon")

	if latStr != "" && lonStr != "" {
		var parseErr error
		lat, parseErr = strconv.ParseFloat(latStr, 64)
		if parseErr != nil {
			app.BadRequest(w, r, "Invalid latitude value.")
			return
		}
		lon, parseErr = strconv.ParseFloat(lonStr, 64)
		if parseErr != nil {
			app.BadRequest(w, r, "Invalid longitude value.")
			return
		}
	} else if address != "" {
		lat, lon, err = geocode(address)
		if err != nil {
			app.BadRequest(w, r, "Could not find that location. Please try a different address or postcode.")
			return
		}
	} else {
		app.BadRequest(w, r, "Please provide an address, postcode, or location coordinates.")
		return
	}

	radius := 500 // default 500m
	if radiusStr := r.Form.Get("radius"); radiusStr != "" {
		if v, parseErr := strconv.Atoi(radiusStr); parseErr == nil {
			radius = v
		}
		if radius < 100 {
			radius = 100
		}
		if radius > 5000 {
			radius = 5000
		}
	}

	// Find nearby places
	results, err := findNearbyPlaces(lat, lon, radius)
	if err != nil {
		app.Log("places", "Nearby error: %v", err)
		app.ServerError(w, r, "Nearby search failed. Please try again.")
		return
	}

	// Consume quota after successful operation
	if useFree {
		wallet.UseFreeSearch(acc.ID)
	} else if cost > 0 {
		wallet.DeductCredits(acc.ID, cost, wallet.OpPlacesNearby, map[string]interface{}{
			"lat": lat, "lon": lon, "radius": radius,
		})
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"results": results,
			"count":   len(results),
			"lat":     lat,
			"lon":     lon,
			"radius":  radius,
		})
		return
	}

	// Render results page
	label := address
	if label == "" {
		label = fmt.Sprintf("%.4f, %.4f", lat, lon)
	}
	html := renderNearbyResults(label, lat, lon, radius, results)
	app.Respond(w, r, app.Response{
		Title:       "Nearby - " + label,
		Description: fmt.Sprintf("Places near %s", label),
		HTML:        html,
	})
}

// renderPlacesPage renders the main places page HTML with map
func renderPlacesPage(r *http.Request) string {
	_, acc := auth.TrySession(r)
	isLoggedIn := acc != nil

	authNote := ""
	if !isLoggedIn {
		authNote = `<p class="text-muted">Search (5p) and nearby (2p) require an account. <a href="/login">Login</a> or <a href="/signup">sign up</a> to search.</p>`
	}

	// Build cities section HTML
	cityCardsHTML := renderCitiesSection()

	// Build city markers JSON for the map
	cityMarkersJS := renderCityMarkersJS()

	return fmt.Sprintf(`<div class="places-page">
%s
<div class="places-forms">
  <div class="card">
    <h3>Search Places</h3>
    <form action="/places/search" method="POST" class="places-search-form">
      <input type="text" name="q" placeholder="Search for a place, address or postcode" required>
      <input type="text" name="near" id="search-near" placeholder="Near (optional location)">
      <input type="hidden" name="near_lat" id="search-near-lat">
      <input type="hidden" name="near_lon" id="search-near-lon">
      <button type="button" onclick="useSearchLocation()" class="btn-secondary">Use My Location</button>
      <button type="submit">Search <span class="cost-badge">5p</span></button>
    </form>
  </div>
  <div class="card">
    <h3>Nearby</h3>
    <form action="/places/nearby" method="POST" class="places-nearby-form" id="nearby-form">
      <input type="text" name="address" id="nearby-address" placeholder="Address or postcode">
      <input type="hidden" name="lat" id="nearby-lat">
      <input type="hidden" name="lon" id="nearby-lon">
      <select name="radius">
        <option value="250">250m</option>
        <option value="500" selected>500m</option>
        <option value="1000">1km</option>
        <option value="2000">2km</option>
        <option value="5000">5km</option>
      </select>
      <button type="submit">Search Nearby <span class="cost-badge">2p</span></button>
      <button type="button" onclick="usePlacesLocation()" class="btn-secondary">Use My Location</button>
    </form>
  </div>
</div>
<div id="places-map" style="height:400px;width:100%%;margin:16px 0;border-radius:8px;"></div>
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin="">
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV/XN/WPeE=" crossorigin=""></script>
<script>
(function() {
  var map = L.map('places-map').setView([20, 0], 2);
  L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>'
  }).addTo(map);
  window._placesMap = map;

  // City markers
  var cities = %s;
  cities.forEach(function(c) {
    L.marker([c.lat, c.lon]).addTo(map).bindPopup(
      '<b>' + c.name + '</b><br>' +
      '<a href="#" onclick="goToCity(' + JSON.stringify(c.name) + ',' + c.lat + ',' + c.lon + ');return false;">Search nearby &rarr;</a>'
    );
  });
})();

function goToCity(name, lat, lon) {
  document.getElementById('nearby-lat').value = lat;
  document.getElementById('nearby-lon').value = lon;
  document.getElementById('nearby-address').value = '';
  document.getElementById('nearby-address').placeholder = name;
  if (window._placesMap) {
    window._placesMap.setView([lat, lon], 13);
  }
  document.getElementById('nearby-form').scrollIntoView({behavior:'smooth'});
}

function usePlacesLocation() {
  if (!navigator.geolocation) {
    alert('Geolocation is not supported by your browser');
    return;
  }
  navigator.geolocation.getCurrentPosition(function(pos) {
    var lat = pos.coords.latitude;
    var lon = pos.coords.longitude;
    document.getElementById('nearby-lat').value = lat;
    document.getElementById('nearby-lon').value = lon;
    document.getElementById('nearby-address').value = '';
    document.getElementById('nearby-address').placeholder = lat.toFixed(4) + ', ' + lon.toFixed(4);
    if (window._placesMap) {
      window._placesMap.setView([lat, lon], 13);
      L.marker([lat, lon]).addTo(window._placesMap).bindPopup('Your location').openPopup();
    }
  }, function(err) {
    alert('Could not get your location: ' + err.message);
  });
}

function useSearchLocation() {
  if (!navigator.geolocation) {
    alert('Geolocation is not supported by your browser');
    return;
  }
  navigator.geolocation.getCurrentPosition(function(pos) {
    var lat = pos.coords.latitude;
    var lon = pos.coords.longitude;
    document.getElementById('search-near-lat').value = lat;
    document.getElementById('search-near-lon').value = lon;
    document.getElementById('search-near').value = '';
    document.getElementById('search-near').placeholder = lat.toFixed(4) + ', ' + lon.toFixed(4);
    if (window._placesMap) {
      window._placesMap.setView([lat, lon], 13);
    }
  }, function(err) {
    alert('Could not get your location: ' + err.message);
  });
}
</script>
%s
</div>`, authNote, cityMarkersJS, cityCardsHTML)
}

// renderCitiesSection renders a grid of city cards
func renderCitiesSection() string {
	cs := Cities()
	if len(cs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<h3>Browse by City</h3><div class="city-grid">`)
	for _, c := range cs {
		sb.WriteString(fmt.Sprintf(
			`<a href="#" onclick="goToCity(%s,%f,%f);return false;" class="city-link">%s <span class="text-muted" style="font-size:0.8em;">%s</span></a>`,
			jsonStr(c.Name), c.Lat, c.Lon,
			escapeHTML(c.Name), escapeHTML(c.Country),
		))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// renderCityMarkersJS returns a JSON array literal of city definitions for use in map JS
func renderCityMarkersJS() string {
	cs := Cities()
	if len(cs) == 0 {
		return "[]"
	}
	type cityJS struct {
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	}
	arr := make([]cityJS, len(cs))
	for i, c := range cs {
		arr[i] = cityJS{Name: c.Name, Lat: c.Lat, Lon: c.Lon}
	}
	b, _ := json.Marshal(arr)
	return string(b)
}

// renderSearchResults renders search results with map
func renderSearchResults(query string, places []*Place, nearLat, nearLon float64) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<div class="places-page">
<p><a href="/places">&larr; Back to Places</a></p>
<h2>Results for &#34;%s&#34;</h2>`, escapeHTML(query)))

	if len(places) == 0 {
		sb.WriteString(`<p class="text-muted">No places found. Try a different search.</p>`)
	} else {
		sb.WriteString(fmt.Sprintf(`<p class="text-muted">%d result(s) found</p>`, len(places)))
		sb.WriteString(`<div id="places-map" style="height:400px;width:100%;margin:16px 0;border-radius:8px;"></div>`)
		sb.WriteString(renderMapScript(places, nearLat, nearLon, 0))
	}

	sb.WriteString(`<div class="places-results">`)
	for _, p := range places {
		sb.WriteString(renderPlaceCard(p))
	}
	sb.WriteString(`</div></div>`)

	return sb.String()
}

// renderNearbyResults renders nearby search results with map
func renderNearbyResults(label string, lat, lon float64, radius int, places []*Place) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<div class="places-page">
<p><a href="/places">&larr; Back to Places</a></p>
<h2>Nearby &#34;%s&#34;</h2>
<p class="text-muted">Within %dm radius</p>`, escapeHTML(label), radius))

	if len(places) == 0 {
		sb.WriteString(`<p class="text-muted">No places found nearby. Try increasing the radius.</p>`)
	} else {
		sb.WriteString(fmt.Sprintf(`<p class="text-muted">%d place(s) found</p>`, len(places)))
		sb.WriteString(`<div id="places-map" style="height:400px;width:100%;margin:16px 0;border-radius:8px;"></div>`)
		sb.WriteString(renderMapScript(places, lat, lon, radius))
	}

	sb.WriteString(`<div class="places-results">`)
	for _, p := range places {
		sb.WriteString(renderPlaceCard(p))
	}
	sb.WriteString(`</div></div>`)

	return sb.String()
}

// renderPlaceCard renders a single place card
func renderPlaceCard(p *Place) string {
	cat := ""
	if p.Category != "" {
		label := strings.ReplaceAll(p.Category, "_", " ")
		if p.Type != "" && p.Type != p.Category {
			label += " · " + strings.ReplaceAll(p.Type, "_", " ")
		}
		cat = fmt.Sprintf(` <span class="place-category">%s</span>`, escapeHTML(label))
	}
	addr := p.Address
	if addr == "" && p.DisplayName != "" {
		addr = p.DisplayName
	}
	addrHTML := ""
	if addr != "" {
		addrHTML = fmt.Sprintf(`<p class="place-address text-muted">%s</p>`, escapeHTML(addr))
	}
	distHTML := ""
	if p.Distance > 0 {
		if p.Distance >= 1000 {
			distHTML = fmt.Sprintf(`<p class="text-muted" style="font-size:0.85em;">%.1f km away</p>`, p.Distance/1000)
		} else {
			distHTML = fmt.Sprintf(`<p class="text-muted" style="font-size:0.85em;">%.0f m away</p>`, p.Distance)
		}
	}
	mapsURL := fmt.Sprintf("https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f#map=16/%.6f/%.6f", p.Lat, p.Lon, p.Lat, p.Lon)
	return fmt.Sprintf(`<div class="card place-card">
  <h4>%s%s</h4>
  %s%s
  <p class="text-muted" style="font-size:0.85em;"><a href="%s" target="_blank" rel="noopener">View on map</a> &middot; %.4f, %.4f</p>
</div>`, escapeHTML(p.Name), cat, addrHTML, distHTML, mapsURL, p.Lat, p.Lon)
}

// placePopupHTML builds an HTML string for a Leaflet map popup for p.
func placePopupHTML(p *Place) string {
	cat := strings.ReplaceAll(p.Category, "_", " ")
	if p.Type != "" && p.Type != p.Category {
		cat += " · " + strings.ReplaceAll(p.Type, "_", " ")
	}
	popup := "<b>" + escapeHTML(p.Name) + "</b>"
	if cat != "" {
		popup += "<br><em>" + escapeHTML(cat) + "</em>"
	}
	addr := p.Address
	if addr == "" {
		addr = p.DisplayName
	}
	if addr != "" {
		popup += "<br>" + escapeHTML(addr)
	}
	return popup
}

// renderMapScript generates Leaflet map JavaScript
// If lat/lon/radius > 0, a circle is drawn at the center point
func renderMapScript(places []*Place, centerLat, centerLon float64, radius int) string {
	if len(places) == 0 && centerLat == 0 {
		return ""
	}

	mapLat := centerLat
	mapLon := centerLon
	zoom := 14

	if centerLat == 0 && len(places) > 0 {
		mapLat = places[0].Lat
		mapLon = places[0].Lon
	}

	circleJS := ""
	if radius > 0 {
		circleJS = fmt.Sprintf(
			`L.circle([%f,%f],{radius:%d,color:'#3388ff',fillOpacity:0.1}).addTo(map);`,
			centerLat, centerLon, radius,
		)
		zoom = 15
	}

	// When showing multiple markers without a fixed center, collect into an
	// array so we can call fitBounds afterwards.
	var markersJS strings.Builder
	fitBoundsJS := ""
	if radius == 0 && len(places) > 1 {
		markersJS.WriteString("var markers = [];\n  ")
		for _, p := range places {
			markersJS.WriteString(fmt.Sprintf(
				`markers.push(L.marker([%f,%f]).addTo(map).bindPopup(%s));`,
				p.Lat, p.Lon, jsonStr(placePopupHTML(p)),
			))
		}
		fitBoundsJS = `var bounds = L.latLngBounds(markers.map(function(m){return m.getLatLng();}));
  map.fitBounds(bounds, {padding:[40,40]});`
	} else {
		for _, p := range places {
			markersJS.WriteString(fmt.Sprintf(
				`L.marker([%f,%f]).addTo(map).bindPopup(%s);`,
				p.Lat, p.Lon, jsonStr(placePopupHTML(p)),
			))
		}
	}

	return fmt.Sprintf(`<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin="">
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV/XN/WPeE=" crossorigin=""></script>
<script>
(function() {
  var map = L.map('places-map').setView([%f,%f],%d);
  L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png',{maxZoom:19,attribution:'&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>'}).addTo(map);
  %s
  %s
  %s
})();
</script>`, mapLat, mapLon, zoom, circleJS, markersJS.String(), fitBoundsJS)
}

// jsonStr returns a JSON-encoded string for use in JavaScript
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// escapeHTML escapes HTML special characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
