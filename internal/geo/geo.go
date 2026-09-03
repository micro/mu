// Package geo is where a place name becomes coordinates, and coordinates become
// a distance.
//
// It exists because two services needed the same thing and one of them took it
// from the other. Flights needed to turn "Camden, London" into a point, places
// already knew how, so flights imported places — a service reaching sideways
// into another service, which makes the pair a unit that has to be understood
// together and moved together. Shared functionality belongs underneath both, not
// inside one of them.
//
// The geocoder is Nominatim: free, keyless, and asking for restraint in return.
// Restraint is easier to honour from one place, which is the second reason this
// is a package rather than a copied function — one User-Agent, one cache, one
// thing to slow down if OpenStreetMap ever asks.
//
// The distance maths was duplicated outright: places had haversine in metres,
// flights had the same formula in nautical miles, and a third service would have
// written a third. It is the same great circle either way.
package geo

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// nominatimURL is a variable so tests can point it at a stub. A test that
// reaches the real service is slow, flaky and rude to a free provider.
var nominatimURL = "https://nominatim.openstreetmap.org/search"

// userAgent identifies Mu. Nominatim's usage policy asks for one, and an
// anonymous flood is what gets a free service closed to everybody.
const userAgent = "Mu/1.0 (https://micro.mu)"

// Point is somewhere on the Earth.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

var (
	cacheMu sync.Mutex
	cache   = map[string]Point{}
)

// maxCacheEntries bounds the cache. A caller asking for a thousand made-up
// place names must not be able to grow it without limit.
const maxCacheEntries = 2000

// Remember puts a known place in the cache, so nothing goes and asks for it.
//
// For tests in other packages. nominatimURL above is a variable so this
// package's own tests can point it at a stub, and a test one package up cannot
// reach it — so service/routes geocoded London landmarks over the real network
// on every run: slow, rude to a free provider, and rate-limited into failure
// the moment more than one test in a package wanted a name resolved. That is
// what "Heathrow Airport — could not be found" was, in a suite that passed when
// the same test ran alone.
//
// A seam and not a stub. What those tests are about is what the code does with
// two points, and having to stand up an HTTP server to say where Camden is puts
// the fixture further from the test than the fact it encodes.
func Remember(address string, lat, lon float64) {
	if address == "" {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if len(cache) >= maxCacheEntries {
		cache = map[string]Point{}
	}
	cache[address] = Point{Lat: lat, Lon: lon}
}

// Geocode resolves an address, postcode or place name to coordinates.
//
// Cached for the life of the process. Towns do not move, and the same name is
// asked for by the page, the card and the agent within seconds of each other.
func Geocode(address string) (lat, lon float64, err error) {
	if address == "" {
		return 0, 0, fmt.Errorf("geo: nothing to locate")
	}
	cacheMu.Lock()
	if p, ok := cache[address]; ok {
		cacheMu.Unlock()
		return p.Lat, p.Lon, nil
	}
	cacheMu.Unlock()

	req, err := http.NewRequest(http.MethodGet, nominatimURL+"?q="+url.QueryEscape(address)+"&format=json&limit=1", nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", userAgent)

	rsp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("geo: nominatim returned %s", rsp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, 1<<20))
	if err != nil {
		return 0, 0, err
	}
	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return 0, 0, fmt.Errorf("could not geocode address: %s", address)
	}
	lat, err1 := strconv.ParseFloat(results[0].Lat, 64)
	lon, err2 := strconv.ParseFloat(results[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("could not geocode address: %s", address)
	}

	cacheMu.Lock()
	if len(cache) >= maxCacheEntries {
		cache = map[string]Point{}
	}
	cache[address] = Point{Lat: lat, Lon: lon}
	cacheMu.Unlock()
	return lat, lon, nil
}

// Earth's mean radius, in each unit something here wants an answer in.
const (
	earthM  = 6371000.0
	earthKM = 6371.0
	earthNM = 3440.065
)

// DistanceM is the great-circle distance in metres.
func DistanceM(lat1, lon1, lat2, lon2 float64) float64 {
	return earthM * central(lat1, lon1, lat2, lon2)
}

// DistanceKM is the great-circle distance in kilometres.
func DistanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	return earthKM * central(lat1, lon1, lat2, lon2)
}

// DistanceNM is the great-circle distance in nautical miles, which is what
// anything to do with aircraft or ships is measured in.
func DistanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	return earthNM * central(lat1, lon1, lat2, lon2)
}

// central is the angle subtended at the Earth's centre by two points —
// haversine, without picking a unit. Multiplying it by a radius is what turns it
// into a distance, and that is the only part the three functions above disagree
// about.
func central(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := rad(lat1), rad(lat2)
	dp, dl := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// Bearing is the initial great-circle bearing from one point to another, in
// degrees clockwise from true north.
//
// Initial, and it matters: follow a great circle any distance and the bearing
// changes under you. Over the tens of miles this is used for that is invisible;
// across an ocean it is not.
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := rad(lat1), rad(lat2)
	dl := rad(lon2 - lon1)
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }
