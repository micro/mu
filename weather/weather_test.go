package weather

import (
	"testing"
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
