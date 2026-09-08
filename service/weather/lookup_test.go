package weather

import (
	"context"
	"fmt"
	"mu/internal/auth"
	"mu/internal/service"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLookupAsksForMissingPlace(t *testing.T) {
	var rsp ForecastResponse
	if err := (Server{}).Lookup(context.Background(), &LookupRequest{Focus: "rain"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Summary, "Which town or city") {
		t.Fatal(rsp.Summary)
	}
}
func TestLookupReportsUnknownPlace(t *testing.T) {
	stubGeocoder(t, nil)
	var rsp ForecastResponse
	if err := (Server{}).Lookup(context.Background(), &LookupRequest{Place: "Missing"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Summary, "couldn't find") {
		t.Fatal(rsp.Summary)
	}
}

func TestLookupWeatherAndRainMatrix(t *testing.T) {
	cases := []struct {
		name, focus  string
		day          *DailyItem
		want, absent string
	}{
		{"general", "", &DailyItem{MaxTempC: 20, MinTempC: 12, Description: "Cloudy"}, "Cloudy", "Chance of rain"},
		{"rain amount", "rain", &DailyItem{RainMM: 3.5, WillRain: true, RainChance: 80}, "3.5 mm", "No rain"},
		{"probability only", "rain", &DailyItem{WillRain: true, RainChance: 60}, "Chance of rain: 60%", "0.0 mm"},
		{"low probability", "rain", &DailyItem{RainChance: 20}, "Chance of rain: 20%", "No rain"},
		{"rain flag only", "rain", &DailyItem{WillRain: true}, "Rain is possible today", "0.0 mm"},
		{"no rain indicated", "rain", &DailyItem{Description: "Sunny"}, "No rain is indicated", "Rain is forecast"},
		{"missing today", "rain", nil, "rain forecast is unavailable", "No rain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCache()
			t.Cleanup(resetCache)
			stubGeocoder(t, []Place{{Name: "London", Country: "United Kingdom", Lat: 51.5, Lon: -0.1}})
			forecast := &WeatherForecast{Location: "London", Current: &CurrentConditions{TempC: 18, Description: "Cloudy"}, ObservedAt: time.Now().UTC()}
			if tc.day != nil {
				day := *tc.day
				day.Date = time.Now().UTC()
				forecast.DailyItems = []DailyItem{day}
			}
			storeForecast(51.5, -0.1, forecast)
			var rsp ForecastResponse
			if err := (Server{}).Lookup(context.Background(), &LookupRequest{Place: "London", Focus: tc.focus}, &rsp); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rsp.Summary, tc.want) || strings.Contains(rsp.Summary, tc.absent) {
				t.Fatalf("unexpected summary: %s", rsp.Summary)
			}
			if strings.Contains(rsp.Summary, "Calendar rule") || strings.Contains(rsp.Summary, "Current request date:") {
				t.Fatal("leaked model instructions")
			}
			if !strings.Contains(rsp.Summary, "United Kingdom") {
				t.Fatal("resolved location not identified")
			}
		})
	}
}

func TestLookupReportsGeocoderFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server failure", 503, `unavailable`}, {"invalid json", 200, `{broken`}, {"wrong schema", 200, `{"results":"bad"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer srv.Close()
			old := geocodeURL
			geocodeURL = srv.URL
			defer func() { geocodeURL = old }()
			var rsp ForecastResponse
			if err := (Server{}).Lookup(context.Background(), &LookupRequest{Place: "London"}, &rsp); err == nil || rsp.Summary != "" {
				t.Fatalf("failure treated as weather: %q %v", rsp.Summary, err)
			}
		})
	}
}

func TestLookupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	old := geocodeURL
	geocodeURL = "http://127.0.0.1:1"
	defer func() { geocodeURL = old }()
	var rsp ForecastResponse
	if err := (Server{}).Lookup(ctx, &LookupRequest{Place: "London"}, &rsp); err == nil {
		t.Fatal("cancelled lookup succeeded")
	}
}

func TestLookupUsesDatedRowsAndBoundsOutput(t *testing.T) {
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	f := &WeatherForecast{DailyItems: []DailyItem{{Date: at.AddDate(0, 0, -1), RainMM: 99}}}
	for i := 1; i <= 7; i++ {
		f.DailyItems = append(f.DailyItems, DailyItem{Date: at.AddDate(0, 0, i), Description: fmt.Sprintf("day-%d", i)})
	}
	text := lookupForecastText(f, "London", "rain", at)
	if !strings.Contains(text, "rain forecast is unavailable") || strings.Contains(text, "99") || strings.Contains(text, "day-6") || strings.Contains(text, "Forecast updated") {
		t.Fatalf("invented current data or unbounded rows: %s", text)
	}
	if !strings.Contains(text, "Current conditions are unavailable") || !strings.Contains(text, "day-5") {
		t.Fatal(text)
	}
}

func TestLookupRejectsInvalidInputsBeforeFetching(t *testing.T) {
	old := geocodeURL
	geocodeURL = "http://127.0.0.1:1"
	defer func() { geocodeURL = old }()
	for _, req := range []LookupRequest{{Place: strings.Repeat("a", 201)}, {Place: "London", Focus: "unknown"}} {
		var rsp ForecastResponse
		err := (Server{}).Lookup(context.Background(), &req, &rsp)
		if err == nil || strings.Contains(err.Error(), "connect") {
			t.Fatalf("input was not rejected before network: %v", err)
		}
	}
}
func TestLookupRejectsInvalidResolvedCoordinates(t *testing.T) {
	stubGeocoder(t, []Place{{Name: "Broken", Lat: 1000, Lon: 0}})
	var rsp ForecastResponse
	err := (Server{}).Lookup(context.Background(), &LookupRequest{Place: "Broken"}, &rsp)
	if err == nil || !strings.Contains(err.Error(), "invalid coordinates") {
		t.Fatalf("accepted invalid location: %v", err)
	}
}

func TestLookupUsesOnlyTheCallersSavedLocation(t *testing.T) {
	oldTransport, oldClient, oldMeteo := http.DefaultTransport, httpClient, openMeteoClient
	local := localWeatherTransport{base: oldTransport}
	http.DefaultTransport = local
	httpClient = &http.Client{Transport: local}
	openMeteoClient = &http.Client{Transport: local}
	t.Cleanup(func() { http.DefaultTransport = oldTransport; httpClient = oldClient; openMeteoClient = oldMeteo })

	resetCache()
	t.Cleanup(resetCache)
	for _, acc := range []*auth.Account{{ID: "weather_saved", Place: "Hampton", Lat: 51.4, Lon: -0.3, Zone: "Europe/London"}, {ID: "weather_named", Place: "Oxford"}, {ID: "weather_unknown"}} {
		if err := auth.Create(acc); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { auth.DeleteAccount(acc.ID) })
	}
	stubGeocoder(t, []Place{{Name: "Oxford", Lat: 51.75, Lon: -1.25}})
	for _, loc := range []struct {
		lat, lon float64
		temp     float64
	}{{51.4, -0.3, 18}, {51.75, -1.25, 12}} {
		storeForecast(loc.lat, loc.lon, &WeatherForecast{Current: &CurrentConditions{TempC: loc.temp, Description: "Cloudy"}, ObservedAt: time.Now()})
	}
	for _, tc := range []struct{ owner, place, want string }{{"weather_saved", "", "Hampton"}, {"weather_named", "", "Oxford"}, {"weather_saved", "Oxford", "Oxford"}, {"weather_unknown", "", "Which town or city"}, {"", "", "Which town or city"}} {
		var rsp ForecastResponse
		if err := (Server{}).Lookup(service.WithAccount(context.Background(), tc.owner), &LookupRequest{Place: tc.place}, &rsp); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(rsp.Summary, tc.want) {
			t.Fatalf("%+v: %s", tc, rsp.Summary)
		}
	}
}

type localWeatherTransport struct{ base http.RoundTripper }

func (l localWeatherTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ip := net.ParseIP(r.URL.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("test forbids external weather requests")
	}
	return l.base.RoundTrip(r)
}

func TestLookupRainUsesLocalCalendarDate(t *testing.T) {
	for _, offset := range []int{-7 * 3600, 14 * 3600} {
		at := time.Date(2026, 9, 8, 0, 30, 0, 0, time.FixedZone("local", offset))
		if offset < 0 {
			at = time.Date(2026, 9, 8, 23, 30, 0, 0, at.Location())
		}
		f := &WeatherForecast{DailyItems: []DailyItem{
			{Date: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)},
			{Date: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), RainMM: 12},
		}}
		text := lookupForecastText(f, "Home", "rain", at)
		if !strings.Contains(text, "No rain is indicated in today’s forecast.") {
			t.Fatalf("offset %d: %s", offset, text)
		}
	}
}

func TestLookupTomorrowSelectsRequestedDate(t *testing.T) {
	at := time.Date(2026, 9, 9, 0, 30, 0, 0, time.FixedZone("home", 14*3600))
	f := &WeatherForecast{Current: &CurrentConditions{TempC: 99}, DailyItems: []DailyItem{
		{Date: at.AddDate(0, 0, -1), Description: "old"},
		{Date: at, MinTempC: 11, MaxTempC: 18, Description: "Rain", RainMM: 4, RainChance: 80},
		{Date: at.AddDate(0, 0, 1), Description: "later"},
	}}
	text := lookupDayText(f, "Hampton", at)
	if !strings.Contains(text, "Wednesday 9 September") || !strings.Contains(text, "11–18°C") || !strings.Contains(text, "80%") || strings.Contains(text, "old") || strings.Contains(text, "later") || strings.Contains(text, "99") {
		t.Fatal(text)
	}
	if text = lookupDayText(f, "Hampton", at.AddDate(0, 0, 5)); !strings.Contains(text, "unavailable") {
		t.Fatal(text)
	}
	var rsp ForecastResponse
	if err := (Server{}).Lookup(context.Background(), &LookupRequest{Day: "next week"}, &rsp); err == nil {
		t.Fatal("ignored unsupported date")
	}
}
