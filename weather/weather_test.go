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
