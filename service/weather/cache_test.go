package weather

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// nwsProvider stands up a fake weather provider and counts how many times it is
// asked. Google is the paid one; the free fallback exercises the same code
// path, and what is being measured is how often the path runs at all.
func nwsProvider(t *testing.T) *int {
	t.Helper()
	resetCache()
	t.Setenv("GOOGLE_API_KEY", "")

	calls := 0
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close(); resetCache() })
	restoreNWSBaseURL(t, server.URL+"/points")
	// These tests count provider calls, and Open-Meteo is tried before NWS
	// now. Point it at a dead address of its own so it fails fast without
	// touching this server's counter — what is being measured is the cache,
	// not which provider answered.
	deadOpenMeteo(t)

	// One handler for every coordinate: the count is the point, not the place.
	mux.HandleFunc("/points/", func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"properties":{"forecastHourly":%q,"forecast":%q,
			"relativeLocation":{"properties":{"city":"Testville","state":"TS"}}}}`,
			server.URL+"/hourly", server.URL+"/daily")
	})
	mux.HandleFunc("/hourly", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"properties":{"generatedAt":"2026-08-04T06:00:00Z","periods":[
			{"startTime":"2026-08-04T06:00:00Z","temperature":20,"temperatureUnit":"C","shortForecast":"Sunny"}]}}`))
	})
	mux.HandleFunc("/daily", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"properties":{"generatedAt":"2026-08-04T06:00:00Z","periods":[
			{"name":"Today","startTime":"2026-08-04T06:00:00Z","isDaytime":true,"temperature":20,"temperatureUnit":"C","shortForecast":"Sunny"}]}}`))
	})
	return &calls
}

// The whole point: the same place asked for twice is one call to the provider.
// Every forecast is money, and the home card, the agent and a browser reload
// were each buying their own.
func TestTheSamePlaceIsFetchedOnce(t *testing.T) {
	calls := nwsProvider(t)

	if _, err := FetchWeather(51.5074, -0.1278); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("first fetch made %d calls, want 1", *calls)
	}

	for i := 0; i < 5; i++ {
		if _, err := FetchWeather(51.5074, -0.1278); err != nil {
			t.Fatal(err)
		}
	}
	if *calls != 1 {
		t.Errorf("six requests for one place made %d provider calls, want 1", *calls)
	}
}

// Two people a few metres apart are asking about the same weather.
func TestNearbyCoordinatesShareAnAnswer(t *testing.T) {
	calls := nwsProvider(t)

	FetchWeather(51.5074, -0.1278)
	FetchWeather(51.5081, -0.1275) // ~80 metres away
	if *calls != 1 {
		t.Errorf("neighbouring coordinates made %d calls, want 1", *calls)
	}

	FetchWeather(48.8566, 2.3522) // Paris is not London
	if *calls != 2 {
		t.Errorf("a different city made %d calls in total, want 2", *calls)
	}
}

// Cached, not frozen: after the TTL the provider is asked again.
func TestAStaleForecastIsFetchedAgain(t *testing.T) {
	calls := nwsProvider(t)

	at := time.Now()
	now = func() time.Time { return at }
	t.Cleanup(func() { now = time.Now })

	FetchWeather(51.5074, -0.1278)
	at = at.Add(forecastTTL + time.Minute)
	FetchWeather(51.5074, -0.1278)

	if *calls != 2 {
		t.Errorf("a stale forecast was reused: %d calls, want 2", *calls)
	}
}

// A failed fetch must not be cached, or one provider blip becomes half an hour
// of "weather unavailable".
func TestFailuresAreNotCached(t *testing.T) {
	resetCache()
	t.Setenv("GOOGLE_API_KEY", "")

	fail := true
	calls := 0
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close(); resetCache() })
	restoreNWSBaseURL(t, server.URL+"/points")
	// These tests count provider calls, and Open-Meteo is tried before NWS
	// now. Point it at a dead address of its own so it fails fast without
	// touching this server's counter — what is being measured is the cache,
	// not which provider answered.
	deadOpenMeteo(t)

	mux.HandleFunc("/points/", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"properties":{"forecastHourly":%q,"forecast":%q,
			"relativeLocation":{"properties":{"city":"Testville","state":"TS"}}}}`,
			server.URL+"/hourly", server.URL+"/daily")
	})
	mux.HandleFunc("/hourly", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"properties":{"generatedAt":"2026-08-04T06:00:00Z","periods":[
			{"startTime":"2026-08-04T06:00:00Z","temperature":20,"temperatureUnit":"C","shortForecast":"Sunny"}]}}`))
	})
	mux.HandleFunc("/daily", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"properties":{"generatedAt":"2026-08-04T06:00:00Z","periods":[
			{"name":"Today","startTime":"2026-08-04T06:00:00Z","isDaytime":true,"temperature":20,"temperatureUnit":"C","shortForecast":"Sunny"}]}}`))
	})

	if _, err := FetchWeather(10, 10); err == nil {
		t.Fatal("expected the first fetch to fail")
	}
	before := calls

	fail = false
	if _, err := FetchWeather(10, 10); err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if calls <= before {
		t.Error("a failure was cached, so the retry never reached the provider")
	}
}

// A caller asking for a thousand random places must not grow the cache without
// limit.
func TestTheCacheIsBounded(t *testing.T) {
	resetCache()
	defer resetCache()

	for i := 0; i < maxCacheEntries+50; i++ {
		storeForecast(float64(i)/10, float64(i)/10, &WeatherForecast{Location: "x"})
	}

	cacheMu.Lock()
	n := len(weatherHit)
	cacheMu.Unlock()
	if n > maxCacheEntries {
		t.Errorf("the cache holds %d entries, want at most %d", n, maxCacheEntries)
	}
}

// deadOpenMeteo makes the keyless provider fail immediately, so a test that
// wants NWS gets NWS.
func deadOpenMeteo(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	old := openMeteoBaseURL
	openMeteoBaseURL = srv.URL
	t.Cleanup(func() { openMeteoBaseURL = old })
}
