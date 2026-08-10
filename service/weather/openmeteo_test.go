package weather

// The keyless provider has to actually answer, anywhere.
//
// Without a Google key this used to fall to the US National Weather Service,
// which answers for the United States and returns nothing anywhere else — so a
// fresh instance in London had a weather service that could not tell you the
// weather. Self-hosting starts unconfigured by definition, so the keyless path
// is the one most people meet first.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const openMeteoBody = `{
  "timezone": "Europe/London",
  "current": {"time":"2026-08-10T11:00","temperature_2m":21.4,"apparent_temperature":20.8,
              "relative_humidity_2m":61,"wind_speed_10m":13.2,"weather_code":3,"precipitation":0},
  "hourly": {"time":["2026-08-10T11:00"],"temperature_2m":[21.4],"weather_code":[3],"precipitation":[0]},
  "daily": {"time":["2026-08-10","2026-08-11"],"weather_code":[3,61],
            "temperature_2m_max":[24.1,19.8],"temperature_2m_min":[14.2,13.0],
            "precipitation_sum":[0,4.5]}
}`

func stubOpenMeteo(t *testing.T, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := openMeteoBaseURL
	openMeteoBaseURL = srv.URL
	t.Cleanup(func() { openMeteoBaseURL = old })
}

func TestTheKeylessProviderAnswersOutsideTheUnitedStates(t *testing.T) {
	resetCache()
	t.Setenv("GOOGLE_API_KEY", "")
	stubOpenMeteo(t, openMeteoBody, http.StatusOK)

	f, err := fetchWeather(51.5074, -0.1278) // London
	if err != nil {
		t.Fatalf("a fresh instance cannot fetch the weather: %v", err)
	}
	if f.Source != "Open-Meteo" {
		t.Errorf("source is %q — the keyless path is not Open-Meteo", f.Source)
	}
	if f.Current == nil || f.Current.TempC != 21.4 {
		t.Errorf("current conditions wrong: %+v", f.Current)
	}
	if f.Current.Description != "Overcast" {
		t.Errorf("weather code 3 rendered as %q, want Overcast", f.Current.Description)
	}
	if len(f.DailyItems) != 2 {
		t.Fatalf("got %d daily items, want 2", len(f.DailyItems))
	}
	if f.DailyItems[0].MaxTempC != 24.1 || f.DailyItems[0].MinTempC != 14.2 {
		t.Errorf("day one is %+v", f.DailyItems[0])
	}
	// 4.5mm tomorrow is rain; today's 0 is not.
	if f.DailyItems[0].WillRain || !f.DailyItems[1].WillRain {
		t.Errorf("rain flags wrong: %v then %v", f.DailyItems[0].WillRain, f.DailyItems[1].WillRain)
	}
}

// A response in the wrong shape parses into a forecast of zeroes, which reads
// as a cold clear day rather than as a failure. It has to be treated as one.
func TestAnEmptyResponseIsAFailureNotAColdClearDay(t *testing.T) {
	resetCache()
	t.Setenv("GOOGLE_API_KEY", "")
	stubOpenMeteo(t, `{"timezone":"X"}`, http.StatusOK)

	f, err := fetchWeather(51.5074, -0.1278)
	if err == nil && f != nil && f.Current != nil && f.Current.TempC == 0 &&
		len(f.DailyItems) == 0 {
		t.Error("an empty response became a forecast of zeroes")
	}
}

func TestWeatherCodesCoverTheCommonSky(t *testing.T) {
	for code, want := range map[int]string{
		0: "Clear", 3: "Overcast", 45: "Fog", 61: "Rain",
		71: "Snow", 80: "Rain showers", 95: "Thunderstorm",
	} {
		if got := wmoDescription(code); got != want {
			t.Errorf("code %d = %q, want %q", code, got, want)
		}
		if wmoIcon(code) == "" {
			t.Errorf("code %d has no icon", code)
		}
	}
	if wmoDescription(12345) != "Unknown" {
		t.Error("an unrecognised code should say so rather than guess")
	}
}
