package weather

// What the keyless half must get right.
//
// Most of these are about the difference between a zero and an absence. The
// pollen model covers Europe and returns null everywhere else; the wave model
// has a grid square over Birmingham and every value in it is zero. Reporting
// either as a measurement would give a caller a confident wrong answer, which
// is the failure this whole service exists to avoid.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stub points one of the keyless URLs at a canned body.
func stub(t *testing.T, target *string, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := *target
	*target = srv.URL
	t.Cleanup(func() { *target = old })
}

const europeanAir = `{"timezone":"Europe/London","current":{"time":"2026-08-13T21:00",
 "pm2_5":7.9,"pm10":13.6,"ozone":104,"nitrogen_dioxide":16.6,"sulphur_dioxide":1.3,
 "uv_index":0,"european_aqi":43,"us_aqi":85,
 "alder_pollen":0,"birch_pollen":0,"grass_pollen":3.4,"mugwort_pollen":6.8,
 "olive_pollen":0,"ragweed_pollen":0.1}}`

// Outside the European model every pollen field is null rather than zero.
const foreignAir = `{"timezone":"America/New_York","current":{"time":"2026-08-13T16:00",
 "pm2_5":5.1,"pm10":9.0,"ozone":72,"nitrogen_dioxide":8.2,"sulphur_dioxide":0.6,
 "uv_index":4.2,"european_aqi":null,"us_aqi":31,
 "alder_pollen":null,"birch_pollen":null,"grass_pollen":null,"mugwort_pollen":null,
 "olive_pollen":null,"ragweed_pollen":null}}`

func TestPollenIsSilentWhereNobodyCountsIt(t *testing.T) {
	stub(t, &airURL, foreignAir, http.StatusOK)

	var rsp AirResponse
	if err := (Server{}).Air(context.Background(), &AirRequest{Lat: 40.7, Lon: -74}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(rsp.Summary), "pollen") {
		t.Errorf("reported pollen where the model does not measure it:\n%s", rsp.Summary)
	}
	// The rest of the reading is still real and must survive.
	if !strings.Contains(rsp.Summary, "US AQI 31") {
		t.Errorf("lost the US index when the European one was absent:\n%s", rsp.Summary)
	}
	if strings.Contains(rsp.Summary, "European AQI") {
		t.Errorf("invented a European index outside Europe:\n%s", rsp.Summary)
	}
}

func TestPollenNamesOnlyWhatIsInTheAir(t *testing.T) {
	stub(t, &airURL, europeanAir, http.StatusOK)

	var rsp AirResponse
	if err := (Server{}).Air(context.Background(), &AirRequest{Lat: 51.5, Lon: -0.12}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"grass low", "mugwort low", "European AQI 43", "moderate"} {
		if !strings.Contains(rsp.Summary, want) {
			t.Errorf("missing %q in:\n%s", want, rsp.Summary)
		}
	}
	// Counts under one grain are not a pollen warning, they are rounding.
	for _, unwanted := range []string{"birch", "alder", "olive", "ragweed"} {
		if strings.Contains(rsp.Summary, unwanted) {
			t.Errorf("listed %q, which was below a single grain:\n%s", unwanted, rsp.Summary)
		}
	}
}

const inlandSea = `{"current":{"time":"2026-08-13T21:30","wave_height":0,"wave_period":0,
 "wave_direction":0,"swell_wave_height":0,"swell_wave_period":0,"sea_surface_temperature":null},
 "daily":{"time":["2026-08-13","2026-08-14"],"wave_height_max":[0,0],
 "wave_period_max":[0,0],"wave_direction_dominant":[0,0]}}`

func TestInlandIsNotAFlatCalm(t *testing.T) {
	stub(t, &marineURL, inlandSea, http.StatusOK)

	var rsp MarineResponse
	err := (Server{}).Marine(context.Background(), &MarineRequest{Lat: 52.48, Lon: -1.90}, &rsp)
	if err == nil {
		t.Fatalf("called a landlocked grid square calm water: %q", rsp.Summary)
	}
	if !strings.Contains(err.Error(), "inland") {
		t.Errorf("unhelpful message for an inland point: %v", err)
	}
}

const realSea = `{"current":{"time":"2026-08-13T21:30","wave_height":0.52,"wave_period":6.1,
 "wave_direction":249,"swell_wave_height":0.4,"swell_wave_period":7.05,"sea_surface_temperature":20.5},
 "daily":{"time":["2026-08-13","2026-08-14"],"wave_height_max":[0.64,0.5],
 "wave_period_max":[7.35,7.8],"wave_direction_dominant":[200,250]}}`

func TestSeaStateReadsAsWeatherNotCoordinates(t *testing.T) {
	stub(t, &marineURL, realSea, http.StatusOK)

	var rsp MarineResponse
	if err := (Server{}).Marine(context.Background(), &MarineRequest{Lat: 50.35, Lon: -4.14}, &rsp); err != nil {
		t.Fatal(err)
	}
	// A bearing of 249 degrees means nothing to a reader; WSW does.
	if !strings.Contains(rsp.Summary, "WSW") {
		t.Errorf("left the wave direction in degrees:\n%s", rsp.Summary)
	}
	if strings.Contains(rsp.Summary, "249") {
		t.Errorf("printed raw degrees:\n%s", rsp.Summary)
	}
	if !strings.Contains(rsp.Summary, "sea 20.5°C") {
		t.Errorf("dropped the sea temperature:\n%s", rsp.Summary)
	}
}

const archiveBody = `{"daily":{"time":["2025-08-01","2025-08-02","2025-08-03"],
 "temperature_2m_max":[19.7,19.9,23.0],"temperature_2m_min":[13.1,13.1,12.8],
 "precipitation_sum":[1.2,0,2.5],"wind_speed_10m_max":[18,14,22]}}`

func TestHistorySummarisesBeforeItLists(t *testing.T) {
	stub(t, &archiveURL, archiveBody, http.StatusOK)

	var rsp HistoryResponse
	if err := (Server{}).History(context.Background(), &HistoryRequest{
		Lat: 51.5, Lon: -0.12, Start: "2025-08-01", End: "2025-08-03"}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Average high 20.9°C", "Total rainfall 3.7mm", "Warmest 23.0°C on 3 Aug", "wettest 2.5mm on 3 Aug"} {
		if !strings.Contains(rsp.Summary, want) {
			t.Errorf("missing %q in:\n%s", want, rsp.Summary)
		}
	}
}

func TestHistoryRefusesRangesItCannotAnswer(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		want       string
	}{
		{"backwards", "2025-09-01", "2025-08-01", "before the start"},
		{"unparseable", "last August", "2025-08-31", "start must be a date"},
		{"too long", "2020-01-01", "2025-01-01", "a year or less"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rsp HistoryResponse
			err := (Server{}).History(context.Background(), &HistoryRequest{
				Lat: 51.5, Lon: -0.12, Start: c.start, End: c.end}, &rsp)
			if err == nil {
				t.Fatalf("accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want something about %q", err, c.want)
			}
		})
	}
}

func TestAnEmptyArchiveSaysWhy(t *testing.T) {
	stub(t, &archiveURL, `{"daily":{"time":[]}}`, http.StatusOK)

	var rsp HistoryResponse
	err := (Server{}).History(context.Background(), &HistoryRequest{
		Lat: 51.5, Lon: -0.12, Start: "2026-08-12", End: "2026-08-13"}, &rsp)
	if err == nil {
		t.Fatal("returned a summary of nothing")
	}
	if !strings.Contains(err.Error(), "behind today") {
		t.Errorf("did not explain the lag: %v", err)
	}
}

func TestUpstreamComplaintsReachTheCaller(t *testing.T) {
	stub(t, &airURL, `{"error":true,"reason":"Cannot initialize WeatherVariable from invalid String value"}`,
		http.StatusBadRequest)

	var rsp AirResponse
	err := (Server{}).Air(context.Background(), &AirRequest{Lat: 51.5, Lon: -0.12}, &rsp)
	if err == nil {
		t.Fatal("swallowed an upstream rejection")
	}
	if !strings.Contains(err.Error(), "WeatherVariable") {
		t.Errorf("replaced the reason with a status code: %v", err)
	}
}

func TestCompassNamesTheEightPoints(t *testing.T) {
	cases := map[float64]string{0: "N", 45: "NE", 90: "E", 180: "S", 270: "W", 349: "N", 360: "N"}
	for deg, want := range cases {
		if got := compass(deg); got != want {
			t.Errorf("compass(%v) = %q, want %q", deg, got, want)
		}
	}
}

func TestAirBandsMatchThePublishedScale(t *testing.T) {
	cases := map[int]string{10: "good", 30: "fair", 50: "moderate", 70: "poor", 95: "very poor", 140: "extremely poor"}
	for aqi, want := range cases {
		if got := aqiWord(aqi); got != want {
			t.Errorf("aqiWord(%d) = %q, want %q", aqi, got, want)
		}
	}
}
