package weather

// What it is doing outside, for a page that must not wait to find out.

import "math"

// Now is the current temperature and description at a location, from the cache
// only, and false when nothing is stored for it.
//
// # Cache only, on purpose
//
// The front door draws in one pass and must not block on a third party. Every
// other line on it — the brief, the prices, the headlines — is read from
// something already in memory, and a weather lookup that went to the network
// would be the one thing on the page able to make it slow, on the one page that
// takes arbitrary traffic.
//
// So this reads what is there and says so when there is nothing. A visitor whose
// location has never been asked about gets no weather line and no delay; the
// first time anything else fetches that location — the weather card, the agent
// answering about it — the cache fills and the line appears on the next visit.
// The same discipline as agent/brief: a page shows what has been computed, it
// does not compute.
func Now(lat, lon float64) (tempC int, description string, ok bool) {
	if lat == 0 && lon == 0 {
		return 0, "", false
	}
	f, hit := cachedForecast(lat, lon)
	if !hit || f == nil || f.Current == nil {
		return 0, "", false
	}
	return int(math.Round(f.Current.TempC)), f.Current.Description, true
}

// Warm fetches a location's forecast into the cache if it is not there.
//
// For a caller that knows where somebody is and would like Now to have an
// answer next time — the front door, on a signed-in account with a saved place.
// Run it in the background: it is a network call, and nothing on the page is
// waiting for it.
func Warm(lat, lon float64) {
	if lat == 0 && lon == 0 {
		return
	}
	if _, ok := cachedForecast(lat, lon); ok {
		return
	}
	if f, err := fetchWeather(lat, lon); err == nil && f != nil {
		storeForecast(lat, lon, f)
	}
}
