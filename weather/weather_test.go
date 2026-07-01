package weather

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestToCelsius_Fahrenheit(t *testing.T) {
	tests := []struct {
		degrees float64
		celsius float64
	}{
		{32, 0},
		{212, 100},
		{98.6, 37},
		{-40, -40},
	}
	for _, tt := range tests {
		got := toCelsius(tt.degrees, "FAHRENHEIT")
		diff := got - tt.celsius
		if diff > 0.01 || diff < -0.01 {
			t.Errorf("toCelsius(%v, FAHRENHEIT) = %v, want ~%v", tt.degrees, got, tt.celsius)
		}
	}
}

func TestToCelsius_AlreadyCelsius(t *testing.T) {
	got := toCelsius(25.0, "CELSIUS")
	if got != 25.0 {
		t.Errorf("toCelsius(25, CELSIUS) = %v, want 25", got)
	}
}

func TestToCelsius_UnknownUnit(t *testing.T) {
	// Unknown unit should return degrees as-is
	got := toCelsius(100.0, "KELVIN")
	if got != 100.0 {
		t.Errorf("toCelsius(100, KELVIN) = %v, want 100", got)
	}
}

func TestValidCoordinates(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lon  float64
		want bool
	}{
		{name: "origin", lat: 0, lon: 0, want: true},
		{name: "north edge", lat: 90, lon: 180, want: true},
		{name: "south edge", lat: -90, lon: -180, want: true},
		{name: "lat too high", lat: 90.1, lon: 0, want: false},
		{name: "lat too low", lat: -90.1, lon: 0, want: false},
		{name: "lon too high", lat: 0, lon: 180.1, want: false},
		{name: "lon too low", lat: 0, lon: -180.1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validCoordinates(tt.lat, tt.lon); got != tt.want {
				t.Fatalf("validCoordinates(%v, %v) = %v, want %v", tt.lat, tt.lon, got, tt.want)
			}
		})
	}
}

func TestForecastTextInvalidCoordinatesDoesNotFetch(t *testing.T) {
	got := ForecastText(91, 0)
	want := "Weather is unavailable because the requested coordinates are invalid."
	if got != want {
		t.Fatalf("ForecastText invalid coordinates = %q, want %q", got, want)
	}
}

func TestForecastTextProviderUnavailableIsClear(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	got := ForecastText(51.5074, -0.1278)
	if got != weatherUnavailableMessage {
		t.Fatalf("ForecastText provider unavailable = %q, want %q", got, weatherUnavailableMessage)
	}
}

func TestCardHTMLShowsWeatherUnavailableOnFetchFailure(t *testing.T) {
	got := CardHTML()
	if !strings.Contains(got, "Weather unavailable") {
		t.Fatalf("CardHTML should show a clear unavailable state on fetch failure, got %q", got)
	}
}

func TestRenderWeatherPageGuestShowsAgentPathAndLoginScope(t *testing.T) {
	got := renderWeatherPage(httptest.NewRequest("GET", "/weather", nil))
	for _, want := range []string{
		"Weather forecasts are available through Mu's agent for guests",
		`href="/agent?q=Weather%20in%20San%20Francisco"`,
		"saved location forecasts, pollen, and credit-backed refreshes require an account",
		`href="/login"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("guest weather page missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Please <a href=\"/login\">log in</a> to use Weather") {
		t.Fatalf("guest weather page should not be a dead-end login prompt:\n%s", got)
	}
}

func TestFormatForecastTextAnchorsDatesToRealCalendarRows(t *testing.T) {
	wf := &WeatherForecast{
		Location: "London",
		Current: &CurrentConditions{
			TempC:             18,
			FeelsLikeC:        17,
			Description:       "cloudy",
			Humidity:          70,
			HumidityAvailable: true,
			WindKph:           12,
			WindKphAvailable:  true,
		},
		DailyItems: []DailyItem{
			{Date: time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), MinTempC: 13, MaxTempC: 21, Description: "showers", RainMM: 2, WillRain: true},
			{Date: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), MinTempC: 12, MaxTempC: 20, Description: "bright spells"},
		},
	}

	got := formatForecastText(wf, time.Date(2026, time.June, 30, 9, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"Current request date: Tuesday, 30 June 2026 (2026-06-30, UTC).",
		"Calendar rule: anchor relative words like today/tomorrow to the request date above",
		"Tuesday, 30 June 2026 (2026-06-30): 13–21°C, showers, rain 2mm",
		"Wednesday, 1 July 2026 (2026-07-01): 12–20°C, bright spells",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatForecastText missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "31 June") {
		t.Fatalf("formatForecastText invented impossible calendar date in:\n%s", got)
	}
}

func TestFormatForecastTextTreatsMissingCurrentObservationsAsUnavailable(t *testing.T) {
	wf := &WeatherForecast{
		Location: "New York",
		Current: &CurrentConditions{
			TempC:       24,
			FeelsLikeC:  24,
			Description: "partly cloudy",
		},
	}

	got := formatForecastText(wf, time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC))
	for _, notWant := range []string{"humidity 0%", "wind 0 km/h"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("formatForecastText presented missing reading %q as fact in:\n%s", notWant, got)
		}
	}
	if !strings.Contains(got, "some current observations unavailable") {
		t.Fatalf("formatForecastText should disclose missing current observations in:\n%s", got)
	}
}
