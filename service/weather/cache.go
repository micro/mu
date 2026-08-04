package weather

// A cache in front of the weather provider.
//
// Every forecast is a paid call to Google, and the same forecast was being
// bought over and over: the home card refetches on each render, the agent
// geocodes "London" to the same coordinates for every person who asks, and a
// browser reload is a fresh purchase. Nothing about the weather changes between
// two requests a minute apart.
//
// So: one call per place per half hour, shared by everyone. Coordinates are
// rounded first, because 51.5074,-0.1278 and 51.5081,-0.1275 are the same
// weather and would otherwise be two purchases.
//
// This is a cost control, not a correctness feature. The TTLs are short enough
// that nobody sees stale weather and long enough that a busy instance makes a
// handful of calls an hour instead of one per visitor.

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	// forecastTTL is how long a forecast is reused. Providers update hourly at
	// best; half an hour is invisible to a reader and cuts most of the calls.
	forecastTTL = 30 * time.Minute

	// pollenTTL is longer because pollen is a daily index, not a measurement.
	pollenTTL = 6 * time.Hour

	// coordPrecision rounds to two decimal places — about a kilometre, which is
	// the same weather everywhere it matters.
	coordPrecision = 100

	// maxCacheEntries bounds the cache. A caller asking for a thousand random
	// coordinates must not be able to grow it without limit.
	maxCacheEntries = 500
)

type cacheEntry struct {
	forecast *WeatherForecast
	pollen   []PollenForecast
	stored   time.Time
}

var (
	cacheMu    sync.Mutex
	weatherHit = map[string]cacheEntry{}

	// now is overridable so the expiry can be tested without sleeping.
	now = time.Now
)

// resetCache empties the cache. Tests that stand up a fake provider need it:
// without it, a forecast one test stored is served to the next, which is a real
// property of the cache and would otherwise show up as a mystery.
func resetCache() {
	cacheMu.Lock()
	weatherHit = map[string]cacheEntry{}
	cacheMu.Unlock()
}

// cacheKey rounds a location to the precision the weather is actually reported
// at, so nearby requests share one answer.
func cacheKey(kind string, lat, lon float64) string {
	return fmt.Sprintf("%s:%.2f,%.2f",
		kind,
		math.Round(lat*coordPrecision)/coordPrecision,
		math.Round(lon*coordPrecision)/coordPrecision)
}

// cachedForecast returns a stored forecast for a location, if one is fresh.
func cachedForecast(lat, lon float64) (*WeatherForecast, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e, ok := weatherHit[cacheKey("w", lat, lon)]
	if !ok || e.forecast == nil || now().Sub(e.stored) > forecastTTL {
		return nil, false
	}
	return e.forecast, true
}

func storeForecast(lat, lon float64, f *WeatherForecast) {
	if f == nil {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	evictLocked()
	weatherHit[cacheKey("w", lat, lon)] = cacheEntry{forecast: f, stored: now()}
}

func cachedPollen(lat, lon float64) ([]PollenForecast, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e, ok := weatherHit[cacheKey("p", lat, lon)]
	if !ok || e.pollen == nil || now().Sub(e.stored) > pollenTTL {
		return nil, false
	}
	return e.pollen, true
}

func storePollen(lat, lon float64, p []PollenForecast) {
	if len(p) == 0 {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	evictLocked()
	weatherHit[cacheKey("p", lat, lon)] = cacheEntry{pollen: p, stored: now()}
}

// evictLocked drops expired entries, and then the oldest, until there is room.
// Caller holds the lock.
func evictLocked() {
	if len(weatherHit) < maxCacheEntries {
		return
	}
	for k, e := range weatherHit {
		if now().Sub(e.stored) > pollenTTL {
			delete(weatherHit, k)
		}
	}
	for len(weatherHit) >= maxCacheEntries {
		oldest, at := "", now()
		for k, e := range weatherHit {
			if !e.stored.After(at) {
				oldest, at = k, e.stored
			}
		}
		if oldest == "" {
			return
		}
		delete(weatherHit, oldest)
	}
}
