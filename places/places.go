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
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Type         string  `json:"type"`
	Address      string  `json:"address"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	DisplayName  string  `json:"display_name"`
	Distance     float64 `json:"distance,omitempty"` // metres, set when sorting by proximity
	Phone        string  `json:"phone,omitempty"`
	Website      string  `json:"website,omitempty"`
	OpeningHours string  `json:"opening_hours,omitempty"`
	Cuisine      string  `json:"cuisine,omitempty"`
}

// cache for search results (query -> places)
var searchCache = map[string][]*Place{}
var searchCacheTime = map[string]time.Time{}

const cacheTTL = 1 * time.Hour

// noResultsCacheTTL is the TTL used when a live lookup returned no results.
// Shorter than cacheTTL so that newly-added places are discovered sooner.
const noResultsCacheTTL = 15 * time.Minute

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
	ExtraTags map[string]string `json:"extratags"`
}

// overpassCenter holds the computed centre of a way element
type overpassCenter struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// overpassElement represents a POI from the Overpass API
type overpassElement struct {
	ID     int64             `json:"id"`
	Type   string            `json:"type"`
	Lat    float64           `json:"lat"`
	Lon    float64           `json:"lon"`
	Center *overpassCenter   `json:"center,omitempty"`
	Tags   map[string]string `json:"tags"`
}

type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

// httpClient is the shared HTTP client with timeout.
// The timeout is set to 35s to allow Overpass queries (which use [timeout:25])
// to complete before the client gives up.
var httpClient = &http.Client{Timeout: 35 * time.Second}

// Load initialises the places package
func Load() {
	initCities()
	loaded := loadCityCaches()
	app.Log("places", "Places loaded: %d/%d cities in quadtree", loaded, len(cities))
	loadSavedSearches()
	go fetchMissingCities()
	startHourlyRefresh()
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
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=20&addressdetails=1&extratags=1",
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

		p := &Place{
			ID:          fmt.Sprintf("%d", r.PlaceID),
			Name:        name,
			Category:    r.Class,
			Type:        r.Type,
			Address:     addr,
			Lat:         lat,
			Lon:         lon,
			DisplayName: r.DisplayName,
		}
		if r.ExtraTags != nil {
			p.Phone = r.ExtraTags["phone"]
			if p.Phone == "" {
				p.Phone = r.ExtraTags["contact:phone"]
			}
			p.Website = r.ExtraTags["website"]
			if p.Website == "" {
				p.Website = r.ExtraTags["contact:website"]
			}
			p.OpeningHours = r.ExtraTags["opening_hours"]
			p.Cuisine = r.ExtraTags["cuisine"]
		}
		places = append(places, p)
	}

	// Store in cache
	mutex.Lock()
	searchCache[query] = places
	searchCacheTime[query] = time.Now()
	mutex.Unlock()

	return places, nil
}

// searchNearbyKeyword searches for POIs near a location whose name, category,
// or cuisine matches the given keyword.
// Local caches (SQLite FTS, in-memory quadtree) are always checked first.
// When the local index is stale (older than staleAge) or empty, live sources
// are queried instead: Google Places (when GOOGLE_API_KEY is set), then the
// Overpass name-search API.  A background cache refresh is also triggered so
// that the next search is served from fresh local data.
// Empty results are cached with a shorter TTL to avoid hammering external APIs
// while still letting newly-added places surface quickly.
func searchNearbyKeyword(query string, lat, lon float64, radiusM int) ([]*Place, error) {
	if radiusM <= 0 {
		radiusM = 1000
	}

	cacheKey := fmt.Sprintf("kw:%s:%.4f:%.4f:%d", query, lat, lon, radiusM)

	// 1. Check in-memory result cache (avoids redundant external API calls)
	mutex.RLock()
	if places, ok := searchCache[cacheKey]; ok {
		ttl := cacheTTL
		if len(places) == 0 {
			ttl = noResultsCacheTTL // shorter TTL so new places surface sooner
		}
		if time.Since(searchCacheTime[cacheKey]) < ttl {
			mutex.RUnlock()
			return places, nil
		}
	}
	mutex.RUnlock()

	// Determine whether the local SQLite index for this area is stale.
	areaStale := isAreaStale(lat, lon, radiusM)
	if areaStale {
		// Trigger a background refresh so subsequent searches get fresh data.
		go PrimeCityCache(lat, lon)
	}

	// 2. Try local SQLite FTS index (fast, persisted) — only if data is fresh
	if !areaStale {
		if local, err := searchPlacesFTS(query, lat, lon, radiusM, true); err == nil && len(local) > 0 {
			return local, nil
		}
		// 3. Try in-memory quadtree with keyword filter (covers cuisine too)
		if local := queryLocalByKeyword(query, lat, lon, radiusM); len(local) > 0 {
			return local, nil
		}
	}

	// 4. Try Google Places (when key is configured)
	if googleAPIKey() != "" {
		if gPlaces, err := googleSearch(query, lat, lon, radiusM); err != nil {
			app.Log("places", "google places search error: %v", err)
		} else if len(gPlaces) > 0 {
			mutex.Lock()
			searchCache[cacheKey] = gPlaces
			searchCacheTime[cacheKey] = time.Now()
			mutex.Unlock()
			go indexPlaces(gPlaces)
			return gPlaces, nil
		}
	}

	// 5. Try Overpass live name search (free; finds places not yet in local cache,
	//    e.g. newly opened businesses that have been added to OpenStreetMap).
	if ovPlaces, err := searchOverpassByName(query, lat, lon, radiusM); err != nil {
		app.Log("places", "overpass name search error: %v", err)
	} else if len(ovPlaces) > 0 {
		mutex.Lock()
		searchCache[cacheKey] = ovPlaces
		searchCacheTime[cacheKey] = time.Now()
		mutex.Unlock()
		go indexPlaces(ovPlaces)
		return ovPlaces, nil
	}

	// If the area was stale and live sources returned nothing, fall back to
	// whatever stale local data we have rather than showing nothing.
	if areaStale {
		if local, err := searchPlacesFTS(query, lat, lon, radiusM, true); err == nil && len(local) > 0 {
			return local, nil
		}
		if local := queryLocalByKeyword(query, lat, lon, radiusM); len(local) > 0 {
			return local, nil
		}
	}

	// Cache the empty result so repeated searches don't re-hit external APIs.
	// noResultsCacheTTL (15 min) is used on the next check so new places surface.
	mutex.Lock()
	searchCache[cacheKey] = []*Place{}
	searchCacheTime[cacheKey] = time.Now()
	mutex.Unlock()

	return nil, nil
}

// findNearbyPlaces finds POIs near a location.
// Local caches (SQLite FTS, in-memory quadtree) are always checked first.
// When the local index is stale (older than staleAge) or has too few results,
// live sources are queried: Google Places (when GOOGLE_API_KEY is set).
// A background cache refresh is triggered when stale so the next search is fast.
func findNearbyPlaces(lat, lon float64, radiusM int) ([]*Place, error) {
	// Determine whether the local SQLite index for this area is stale.
	areaStale := isAreaStale(lat, lon, radiusM)
	if areaStale {
		// Trigger a background refresh so subsequent searches get fresh data.
		go PrimeCityCache(lat, lon)
	}

	// 1. Try local SQLite FTS index (fast, persisted) — only if data is fresh
	if !areaStale {
		if local, err := searchPlacesFTS("", lat, lon, radiusM, true); err == nil && len(local) >= minLocalResults {
			return local, nil
		}
		// 2. Try in-memory quadtree
		if local := queryLocal(lat, lon, radiusM); len(local) >= minLocalResults {
			return local, nil
		}
	}

	// 3. Try Google Places (when key is configured) — used when local is stale
	//    or has insufficient coverage.
	if googleAPIKey() != "" {
		if gPlaces, err := googleNearby(lat, lon, radiusM); err != nil {
			app.Log("places", "google places nearby error: %v", err)
		} else if len(gPlaces) > 0 {
			go indexPlaces(gPlaces)
			return gPlaces, nil
		}
	}

	// 4. Fall back to local results (may be stale or fewer than minLocalResults)
	if local, err := searchPlacesFTS("", lat, lon, radiusM, true); err == nil && len(local) > 0 {
		return local, nil
	}
	return queryLocal(lat, lon, radiusM), nil
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
	// Handle sub-routes
	switch r.URL.Path {
	case "/places/search":
		handleSearch(w, r)
		return
	case "/places/nearby":
		handleNearby(w, r)
		return
	case "/places/save":
		handleSaveSearch(w, r)
		return
	case "/places/save/delete":
		handleDeleteSavedSearch(w, r)
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

	// Default: show the places page
	body := renderPlacesPage(r)
	app.Respond(w, r, app.Response{
		Title:       "Places",
		Description: "Search and discover places near you",
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

	// Optional location for proximity-based search
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

	// Parse radius (used when doing a nearby keyword search)
	radiusM := 1000
	if rs := r.Form.Get("radius"); rs != "" {
		if v, perr := strconv.Atoi(rs); perr == nil && v >= 100 && v <= 50000 {
			radiusM = v
		}
	}

	// Perform search: use Overpass keyword search when a location is provided,
	// otherwise fall back to a global Nominatim search.
	// Also prime the local cache in the background so subsequent queries are faster.
	var results []*Place
	if hasNearLoc {
		go PrimeCityCache(nearLat, nearLon)
		results, err = searchNearbyKeyword(query, nearLat, nearLon, radiusM)
	} else {
		results, err = searchNominatim(query)
	}
	if err != nil {
		app.Log("places", "Search error: %v", err)
		app.ServerError(w, r, "Search failed. Please try again.")
		return
	}

	// Apply sort order
	sortBy := r.Form.Get("sort")
	sortPlaces(results, sortBy)

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
	html := renderSearchResults(query, results, hasNearLoc, nearAddr, nearLat, nearLon, sortBy, radiusM)
	app.Respond(w, r, app.Response{
		Title:       "Places - " + query,
		Description: fmt.Sprintf("Search results for %s", query),
		HTML:        html,
	})
}

// handleNearby handles nearby place requests (GET and POST /places/nearby)
func handleNearby(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		r.ParseForm()
		hasLocation := r.Form.Get("address") != "" || r.Form.Get("lat") != "" || r.Form.Get("lon") != ""
		if !hasLocation {
			// No location: redirect to main places page
			http.Redirect(w, r, "/places", http.StatusSeeOther)
			return
		}
		// Has location params in URL: fall through to perform the search
	} else if r.Method != http.MethodPost {
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
		if radius > 50000 {
			radius = 50000
		}
	}

	// Prime the local cache in the background and find nearby places
	go PrimeCityCache(lat, lon)
	results, err := findNearbyPlaces(lat, lon, radius)
	if err != nil {
		app.Log("places", "Nearby error: %v", err)
		app.ServerError(w, r, "Nearby search failed. Please try again.")
		return
	}

	// Apply sort order
	sortBy := r.Form.Get("sort")
	sortPlaces(results, sortBy)

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

// renderPlacesPage renders the main places page HTML
func renderPlacesPage(r *http.Request) string {
	_, acc := auth.TrySession(r)
	isLoggedIn := acc != nil

	authNote := ""
	if !isLoggedIn {
		authNote = `<p class="text-muted">Search requires an account. <a href="/login">Login</a> or <a href="/signup">sign up</a> to search places.</p>`
	}

	savedHTML := ""
	if isLoggedIn {
		savedHTML = renderSavedSearchesSection(acc.ID)
	}

	cityCardsHTML := renderCitiesSection()

	mapHTML := renderIndexMap()

	return fmt.Sprintf(`<div class="places-page">
%s
<div class="places-forms">
<div class="card">
  <h4>Search</h4>
  <p class="text-muted places-form-desc">Find by name or category (e.g. cafe, pharmacy).</p>
  %s
</div>
<div class="card">
  <h4>&#128205; Nearby</h4>
  <p class="text-muted places-form-desc">List places near a location.</p>
  %s
</div>
</div>
%s
%s
%s
%s
</div>`, authNote, renderSearchFormHTML("", "", "", "", "", ""), renderNearbyFormHTML("", "", "", ""), savedHTML, mapHTML, cityCardsHTML, renderPlacesPageJS())
}

// renderNearbyFormHTML returns a form for listing places near a location.
// It is used on the main places page and on the nearby results page.
func renderNearbyFormHTML(address, lat, lon, radius string) string {
	if radius == "" {
		radius = "1000"
	}
	radiusOptions := ""
	for _, opt := range []struct {
		val, label string
	}{
		{"500", "Nearby (~500m)"},
		{"1000", "Walking distance (~1km)"},
		{"2000", "Local area (~2km)"},
		{"5000", "City area (~5km)"},
		{"10000", "Wider city (~10km)"},
		{"25000", "Regional (~25km)"},
		{"50000", "Province (~50km)"},
	} {
		sel := ""
		if opt.val == radius {
			sel = " selected"
		}
		radiusOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, opt.val, sel, opt.label)
	}
	return fmt.Sprintf(`<form id="nearby-form" action="/places/nearby" method="POST">
    <input type="hidden" name="lat" id="nearby-lat" value="%s">
    <input type="hidden" name="lon" id="nearby-lon" value="%s">
    <div class="places-location-row">
      <input type="text" name="address" id="nearby-address" placeholder="Address or postcode" value="%s">
      <a href="#" onclick="useNearbyLocation(this);return false;" class="btn-link">&#128205; Use my location</a>
    </div>
    <div class="places-options-row">
      <select name="radius" id="nearby-radius">%s</select>
    </div>
    <div class="places-actions-row">
      <button type="submit">Find Nearby <span class="cost-badge">2p</span></button>
    </div>
  </form>`,
		escapeHTML(lat), escapeHTML(lon), escapeHTML(address), radiusOptions)
}

// renderIndexMap returns an embedded Leaflet.js map for the main places page.
// It auto-detects the user's current location via geolocation and shows a marker.
// City clicks will recenter this map and update the nearby form.
func renderIndexMap() string {
	return `<div style="height:280px;margin:1rem 0;border-radius:8px;overflow:hidden;position:relative;z-index:0;"><div id="places-index-map" style="height:100%;width:100%;"></div></div>
<script>
var placesIndexMap = null;
var placesIndexMarker = null;
(function(){
  function initIndexMap(lat, lon, zoom) {
    // Default: world overview centred on 20°N 0°E, zoom 2
    lat = lat || 20; lon = lon || 0; zoom = zoom || 2;
    placesIndexMap = L.map('places-index-map').setView([lat, lon], zoom);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
      attribution:'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',maxZoom:19
    }).addTo(placesIndexMap);
    if (zoom > 2) {
      placesIndexMarker = L.marker([lat, lon]).addTo(placesIndexMap).bindPopup('Your location');
    }
  }
  function tryGeolocation() {
    if (!navigator.geolocation) { initIndexMap(); return; }
    navigator.geolocation.getCurrentPosition(function(pos) {
      var lat = pos.coords.latitude, lon = pos.coords.longitude;
      initIndexMap(lat, lon, 13);
      document.getElementById('nearby-lat').value = lat;
      document.getElementById('nearby-lon').value = lon;
      document.getElementById('nearby-address').value = lat.toFixed(4) + ', ' + lon.toFixed(4);
    }, function() { initIndexMap(); }, {timeout: 8000, maximumAge: 300000 /* 5 minutes */});
  }
  function loadLeafletThenInit() {
    var lnk=document.createElement('link');
    lnk.rel='stylesheet';
    lnk.href='https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
    lnk.crossOrigin='anonymous';
    document.head.appendChild(lnk);
    var s=document.createElement('script');
    s.src='https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
    s.crossOrigin='anonymous';
    s.onload=tryGeolocation;
    document.head.appendChild(s);
  }
  if(window.L){ tryGeolocation(); } else { loadLeafletThenInit(); }
})();
function selectCity(lat, lon, name, country) {
  if (placesIndexMap) {
    placesIndexMap.setView([lat, lon], 13);
    if (placesIndexMarker) { placesIndexMap.removeLayer(placesIndexMarker); }
    placesIndexMarker = L.marker([lat, lon]).addTo(placesIndexMap).bindPopup(name).openPopup();
  }
  document.getElementById('nearby-lat').value = lat;
  document.getElementById('nearby-lon').value = lon;
  document.getElementById('nearby-address').value = name + ', ' + country;
  var form = document.getElementById('nearby-form');
  if (form) { form.submit(); }
}
</script>`
}

// renderCitiesSection renders a grid of city cards as direct nearby links
func renderCitiesSection() string {
	cs := Cities()
	if len(cs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<h3>Browse by City</h3><div class="city-grid">`)
	for _, c := range cs {
		sb.WriteString(fmt.Sprintf(
			`<a href="#" onclick="selectCity(%f,%f,%s,%s);return false;" class="city-link">%s <span class="text-muted" style="font-size:0.8em;">%s</span></a>`,
			c.Lat, c.Lon, escapeHTML(jsonStr(c.Name)), escapeHTML(jsonStr(c.Country)),
			escapeHTML(c.Name), escapeHTML(c.Country),
		))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// renderSearchFormHTML returns the shared search form HTML, pre-filled with the given values.
// Used on the main page and on results pages.
func renderSearchFormHTML(q, near, nearLat, nearLon, radius, sortBy string) string {
	if radius == "" {
		radius = "1000"
	}
	radiusOptions := ""
	for _, opt := range []struct {
		val, label string
	}{
		{"500", "Nearby (~500m)"},
		{"1000", "Walking distance (~1km)"},
		{"2000", "Local area (~2km)"},
		{"5000", "City area (~5km)"},
		{"10000", "Wider city (~10km)"},
		{"25000", "Regional (~25km)"},
		{"50000", "Province (~50km)"},
	} {
		sel := ""
		if opt.val == radius {
			sel = " selected"
		}
		radiusOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, opt.val, sel, opt.label)
	}
	sortDistSel, sortNameSel := " selected", ""
	if sortBy == "name" {
		sortDistSel, sortNameSel = "", " selected"
	}
	return fmt.Sprintf(`<form id="places-form" action="/places/search" method="POST">
    <input type="text" name="q" id="places-q" placeholder="What are you looking for?" value="%s">
    <div class="places-location-row">
      <input type="text" name="near" id="places-near" placeholder="Location (optional)" value="%s" oninput="updateNearbyLink()">
      <input type="hidden" name="near_lat" id="places-near-lat" value="%s">
      <input type="hidden" name="near_lon" id="places-near-lon" value="%s">
      <a href="#" onclick="usePlacesLocation(this);return false;" class="btn-link">&#128205; Use my location</a>
    </div>
    <div class="places-options-row">
      <select name="radius" id="places-radius" onchange="updateNearbyLink()">%s</select>
      <select name="sort" id="places-sort">
        <option value="distance"%s>Sort by distance</option>
        <option value="name"%s>Sort by name</option>
      </select>
    </div>
    <div class="places-actions-row">
      <button type="submit">Search <span class="cost-badge">5p</span></button>
    </div>
  </form>`,
		escapeHTML(q), escapeHTML(near), escapeHTML(nearLat), escapeHTML(nearLon),
		radiusOptions, sortDistSel, sortNameSel)
}

// renderSavedSearchesSection returns HTML for the saved searches list
func renderSavedSearchesSection(userID string) string {
	searches := getUserSavedSearches(userID)
	if len(searches) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<div class="card places-saved-card"><h4>Saved searches</h4><ul class="saved-search-list">`)
	for _, s := range searches {
		latStr := fmt.Sprintf("%f", s.Lat)
		lonStr := fmt.Sprintf("%f", s.Lon)
		if s.Lat == 0 {
			latStr = ""
		}
		if s.Lon == 0 {
			lonStr = ""
		}
		sb.WriteString(fmt.Sprintf(
			`<li><a href="#" onclick="runSavedSearch(%s,%s,%s,%s,%s,%s,%s);return false;">%s</a> `+
				`<form style="display:inline" action="/places/save/delete" method="POST">`+
				`<input type="hidden" name="id" value="%s">`+
				`<button type="submit" class="btn-link text-muted" title="Remove">&#x2715;</button></form></li>`,
			escapeHTML(jsonStr(s.Type)), escapeHTML(jsonStr(s.Query)), escapeHTML(jsonStr(s.Location)),
			escapeHTML(jsonStr(latStr)), escapeHTML(jsonStr(lonStr)),
			escapeHTML(jsonStr(fmt.Sprintf("%d", s.Radius))), escapeHTML(jsonStr(s.SortBy)),
			escapeHTML(s.Label), escapeHTML(s.ID),
		))
	}
	sb.WriteString(`</ul></div>`)
	return sb.String()
}

// renderSearchResults renders search results as a list
func renderSearchResults(query string, places []*Place, nearLocation bool, nearAddr string, nearLat, nearLon float64, sortBy string, radiusM int) string {
	var sb strings.Builder

	nearLatStr, nearLonStr := "", ""
	if nearLocation {
		nearLatStr = fmt.Sprintf("%f", nearLat)
		nearLonStr = fmt.Sprintf("%f", nearLon)
	}
	radiusStr := fmt.Sprintf("%d", radiusM)

	sb.WriteString(`<div class="places-page">`)
	sb.WriteString(`<p><a href="/places">&larr; Back to Places</a></p>`)
	sb.WriteString(renderSearchFormHTML(query, nearAddr, nearLatStr, nearLonStr, radiusStr, sortBy))
	sb.WriteString(renderPlacesPageJS())

	sb.WriteString(fmt.Sprintf(`<h2>Results for &#34;%s&#34;</h2>`, escapeHTML(query)))

	if nearLocation {
		locLabel := nearAddr
		if locLabel == "" {
			locLabel = fmt.Sprintf("%.6f, %.6f", nearLat, nearLon)
		}
		sb.WriteString(fmt.Sprintf(`<p class="text-muted">Near <strong>%s</strong></p>`, escapeHTML(locLabel)))
	}

	if len(places) == 0 {
		if nearLocation {
			sb.WriteString(`<p class="text-muted">No places found nearby. Try a larger radius or different search term.</p>`)
		} else {
			sb.WriteString(`<p class="text-muted">No places found. Try a different search term or add a location.</p>`)
		}
	} else {
		sortLabel := sortBy
		if sortLabel == "" || sortLabel == "distance" {
			sortLabel = "distance"
		}
		sb.WriteString(fmt.Sprintf(`<p class="text-muted">%d result(s) &middot; sorted by %s</p>`, len(places), sortLabel))
		sb.WriteString(renderSaveSearchForm("search", query, nearAddr, nearLatStr, nearLonStr, radiusStr, sortBy))
		mapCenterLat, mapCenterLon := nearLat, nearLon
		if !nearLocation && len(places) > 0 {
			mapCenterLat, mapCenterLon = places[0].Lat, places[0].Lon
		}
		if mapCenterLat != 0 || mapCenterLon != 0 {
			sb.WriteString(renderLeafletMap(mapCenterLat, mapCenterLon, places))
		}
	}

	sb.WriteString(`<div class="places-results">`)
	for _, p := range places {
		sb.WriteString(renderPlaceCard(p))
	}
	sb.WriteString(`</div></div>`)

	return sb.String()
}

// renderNearbyResults renders nearby search results as a list
func renderNearbyResults(label string, lat, lon float64, radius int, places []*Place) string {
	var sb strings.Builder

	radiusLabel := radiusName(radius)
	radiusStr := fmt.Sprintf("%d", radius)
	latStr := fmt.Sprintf("%f", lat)
	lonStr := fmt.Sprintf("%f", lon)

	sb.WriteString(`<div class="places-page">`)
	sb.WriteString(`<p><a href="/places">&larr; Back to Places</a></p>`)
	sb.WriteString(renderNearbyFormHTML(label, latStr, lonStr, radiusStr))
	sb.WriteString(renderPlacesPageJS())

	sb.WriteString(`<h2>Nearby</h2>`)
	sb.WriteString(fmt.Sprintf(`<p class="text-muted"><strong>%s</strong> &middot; %s</p>`, escapeHTML(label), escapeHTML(radiusLabel)))

	if len(places) == 0 {
		sb.WriteString(`<p class="text-muted">No places found. Try increasing the radius.</p>`)
	} else {
		sb.WriteString(fmt.Sprintf(`<p class="text-muted">%d place(s) found</p>`, len(places)))
		sb.WriteString(renderSaveSearchForm("nearby", "", label, latStr, lonStr, radiusStr, ""))
		sb.WriteString(renderLeafletMap(lat, lon, places))
		sb.WriteString(renderTypeFilter(places))
	}

	sb.WriteString(`<div class="places-results">`)
	for _, p := range places {
		sb.WriteString(renderPlaceCard(p))
	}
	sb.WriteString(`</div></div>`)

	return sb.String()
}

// renderLeafletMap returns an embedded Leaflet.js map showing the center and place markers
func renderLeafletMap(centerLat, centerLon float64, places []*Place) string {
	var markers []string
	for _, p := range places {
		if p.Lat == 0 && p.Lon == 0 {
			continue
		}
		markers = append(markers, fmt.Sprintf(`{"lat":%f,"lon":%f,"name":%s}`, p.Lat, p.Lon, jsonStr(p.Name)))
	}
	markersJSON := "[" + strings.Join(markers, ",") + "]"
	return fmt.Sprintf(`<div style="height:280px;margin:1rem 0;border-radius:8px;overflow:hidden;position:relative;z-index:0;"><div id="places-map" style="height:100%%;width:100%%;"></div></div>
<script>
(function(){
  function initPlacesMap(){
    var map=L.map('places-map').setView([%f,%f],14);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
      attribution:'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',maxZoom:19
    }).addTo(map);
    var ps=%s;
    var bounds=[];
    ps.forEach(function(p){
      L.marker([p.lat,p.lon]).addTo(map).bindPopup(p.name);
      bounds.push([p.lat,p.lon]);
    });
    if(bounds.length>1){map.fitBounds(bounds,{padding:[30,30],maxZoom:16});}
  }
  if(window.L){
    initPlacesMap();
  } else {
    var lnk=document.createElement('link');
    lnk.rel='stylesheet';
    lnk.href='https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
    lnk.crossOrigin='anonymous';
    document.head.appendChild(lnk);
    var s=document.createElement('script');
    s.src='https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
    s.crossOrigin='anonymous';
    s.onload=initPlacesMap;
    document.head.appendChild(s);
  }
})();
</script>`, centerLat, centerLon, markersJSON)
}

// renderSaveSearchForm returns a small "Save this search" form
func renderSaveSearchForm(searchType, q, near, nearLat, nearLon, radius, sortBy string) string {
	return fmt.Sprintf(`<form action="/places/save" method="POST" style="display:inline-block;margin-bottom:0.5rem;">
  <input type="hidden" name="type" value="%s">
  <input type="hidden" name="q" value="%s">
  <input type="hidden" name="near" value="%s">
  <input type="hidden" name="near_lat" value="%s">
  <input type="hidden" name="near_lon" value="%s">
  <input type="hidden" name="radius" value="%s">
  <input type="hidden" name="sort" value="%s">
  <button type="submit" class="btn-link">&#9733; Save this search</button>
</form>`,
		escapeHTML(searchType), escapeHTML(q), escapeHTML(near),
		escapeHTML(nearLat), escapeHTML(nearLon), escapeHTML(radius), escapeHTML(sortBy))
}

// renderPlacesPageJS returns the shared JavaScript used on all places pages
func renderPlacesPageJS() string {
	return `<script>
function usePlacesLocation(btn) {
  if (!navigator.geolocation) { showToast('Geolocation is not supported by your browser', 'error'); return; }
  if (btn) { btn.textContent = '⏳ Getting location...'; btn.style.pointerEvents = 'none'; }
  navigator.geolocation.getCurrentPosition(function(pos) {
    var lat = pos.coords.latitude, lon = pos.coords.longitude;
    document.getElementById('places-near-lat').value = lat;
    document.getElementById('places-near-lon').value = lon;
    document.getElementById('places-near').value = lat.toFixed(4) + ', ' + lon.toFixed(4);
    if (btn) { btn.innerHTML = '&#128205; Use my location'; btn.style.pointerEvents = ''; }
  }, function(err) {
    if (btn) { btn.innerHTML = '&#128205; Use my location'; btn.style.pointerEvents = ''; }
    showToast('Could not get your location: ' + err.message, 'error');
  }, {timeout: 10000, maximumAge: 60000});
}
function useNearbyLocation(btn) {
  if (!navigator.geolocation) { showToast('Geolocation is not supported by your browser', 'error'); return; }
  if (btn) { btn.textContent = '⏳ Getting location...'; btn.style.pointerEvents = 'none'; }
  navigator.geolocation.getCurrentPosition(function(pos) {
    var lat = pos.coords.latitude, lon = pos.coords.longitude;
    document.getElementById('nearby-lat').value = lat;
    document.getElementById('nearby-lon').value = lon;
    document.getElementById('nearby-address').value = lat.toFixed(4) + ', ' + lon.toFixed(4);
    var form = document.getElementById('nearby-form');
    if (form) { form.submit(); }
  }, function(err) {
    if (btn) { btn.innerHTML = '&#128205; Use my location'; btn.style.pointerEvents = ''; }
    showToast('Could not get your location: ' + err.message, 'error');
  }, {timeout: 10000, maximumAge: 60000});
}
function runSavedSearch(type, q, near, nearLat, nearLon, radius, sortBy) {
  if (type === 'nearby') {
    var u = '/places/nearby?radius=' + radius;
    if (nearLat && nearLon) { u += '&lat=' + nearLat + '&lon=' + nearLon; }
    if (near) { u += '&address=' + encodeURIComponent(near); }
    window.location = u;
  } else {
    document.getElementById('places-q').value = q;
    document.getElementById('places-near').value = near;
    document.getElementById('places-near-lat').value = nearLat;
    document.getElementById('places-near-lon').value = nearLon;
    if (document.getElementById('places-radius')) document.getElementById('places-radius').value = radius;
    if (document.getElementById('places-sort')) document.getElementById('places-sort').value = sortBy;
    document.getElementById('places-form').submit();
  }
}
function filterByType(btn) {
  var cat = btn.dataset.filter || '';
  document.querySelectorAll('.place-card').forEach(function(c) {
    c.style.display = (!cat || c.dataset.category === cat) ? '' : 'none';
  });
  document.querySelectorAll('.type-filter-btn').forEach(function(b) {
    b.classList.toggle('active', b === btn);
  });
}
</script>`
}

// renderPlaceCard renders a single place card with rich details and map links
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
			distHTML = fmt.Sprintf(`<span class="text-muted"> &middot; %.1f km away</span>`, p.Distance/1000)
		} else {
			distHTML = fmt.Sprintf(`<span class="text-muted"> &middot; %.0f m away</span>`, p.Distance)
		}
	}

	gmapsQuery := p.Name
	if p.Address != "" {
		gmapsQuery += ", " + p.Address
	}
	gmapsViewURL := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(gmapsQuery)
	gmapsDirURL := "https://www.google.com/maps/dir/?api=1&destination=" + url.QueryEscape(gmapsQuery)

	extraHTML := ""
	if p.Cuisine != "" {
		extraHTML += fmt.Sprintf(`<p class="place-info text-muted">Cuisine: %s</p>`, escapeHTML(p.Cuisine))
	}
	if p.OpeningHours != "" {
		extraHTML += fmt.Sprintf(`<p class="place-info text-muted">Hours: %s</p>`, escapeHTML(p.OpeningHours))
	} else {
		extraHTML += fmt.Sprintf(`<p class="place-info text-muted">Hours: <a href="%s" target="_blank" rel="noopener noreferrer">check on Google Maps &#8599;</a></p>`, gmapsViewURL)
	}
	if p.Phone != "" {
		extraHTML += fmt.Sprintf(`<p class="place-info"><a href="tel:%s">%s</a></p>`, escapeHTML(p.Phone), escapeHTML(p.Phone))
	}
	if p.Website != "" {
		extraHTML += fmt.Sprintf(`<p class="place-info"><a href="%s" target="_blank" rel="noopener noreferrer">Website &#8599;</a></p>`, escapeHTML(p.Website))
	}

	return fmt.Sprintf(`<div class="card place-card" data-category="%s">
  <h4><a href="%s" target="_blank" rel="noopener">%s</a>%s%s</h4>
  %s%s
  <p class="place-links"><a href="%s" target="_blank" rel="noopener">Get Directions</a></p>
</div>`, escapeHTML(p.Category), gmapsViewURL, escapeHTML(p.Name), cat, distHTML, addrHTML, extraHTML, gmapsDirURL)
}

// renderTypeFilter renders category filter buttons for a set of places.
// Returns an empty string if there are fewer than 2 distinct categories.
func renderTypeFilter(places []*Place) string {
	seen := map[string]struct{}{}
	var cats []string
	for _, p := range places {
		if p.Category != "" {
			if _, ok := seen[p.Category]; !ok {
				seen[p.Category] = struct{}{}
				cats = append(cats, p.Category)
			}
		}
	}
	if len(cats) < 2 {
		return ""
	}
	sort.Strings(cats)
	var sb strings.Builder
	sb.WriteString(`<div class="places-type-filter">`)
	sb.WriteString(`<button class="type-filter-btn active" data-filter="" onclick="filterByType(this)">All</button>`)
	for _, cat := range cats {
		label := strings.ReplaceAll(cat, "_", " ")
		sb.WriteString(fmt.Sprintf(
			`<button class="type-filter-btn" data-filter="%s" onclick="filterByType(this)">%s</button>`,
			escapeHTML(cat), escapeHTML(label),
		))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// sortPlaces sorts places in-place according to sortBy ("name" or "distance").
// Distance sort is a no-op since places are already sorted by distance from the API.
func sortPlaces(places []*Place, sortBy string) {
	if sortBy == "name" {
		sort.Slice(places, func(i, j int) bool {
			return strings.ToLower(places[i].Name) < strings.ToLower(places[j].Name)
		})
	}
}

// radiusName returns a human-friendly name for a radius in metres.
func radiusName(radiusM int) string {
	switch {
	case radiusM <= 500:
		return "Nearby (~500m)"
	case radiusM <= 1000:
		return "Walking distance (~1km)"
	case radiusM <= 2000:
		return "Local area (~2km)"
	case radiusM <= 5000:
		return "City area (~5km)"
	case radiusM <= 10000:
		return "Wider city (~10km)"
	case radiusM <= 25000:
		return "Regional (~25km)"
	default:
		return "Province (~50km)"
	}
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
