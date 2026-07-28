package islam

import (
	"testing"
	"time"
)

// Prayer times are computed locally, so this must hold with no network at all.
func TestGetPrayerTimesIsLocalAndOrdered(t *testing.T) {
	pt, err := GetPrayerTimes(51.5074, -0.1278, "Europe/London")
	if err != nil {
		t.Fatalf("GetPrayerTimes: %v", err)
	}
	for _, e := range pt.Ordered() {
		if e.Time == "" {
			t.Fatalf("%s is empty: %+v", e.Name, pt)
		}
		if _, err := time.Parse("15:04", e.Time); err != nil {
			t.Errorf("%s = %q, not HH:MM", e.Name, e.Time)
		}
	}
	// The day runs in order.
	prev := ""
	for _, e := range pt.Ordered() {
		if prev != "" && e.Time < prev {
			t.Errorf("%s (%s) is before the previous prayer (%s): %+v", e.Name, e.Time, prev, pt)
		}
		prev = e.Time
	}
}

// An unknown or empty timezone must degrade to UTC rather than erroring.
func TestGetPrayerTimesUnknownTimezoneFallsBack(t *testing.T) {
	for _, tz := range []string{"", "Not/AZone"} {
		if _, err := GetPrayerTimes(51.5074, -0.1278, tz); err != nil {
			t.Errorf("tz %q: %v", tz, err)
		}
	}
}

func TestNextPrayerSkipsSunrise(t *testing.T) {
	pt := &PrayerTimes{Fajr: "03:12", Sunrise: "05:18", Dhuhr: "13:07", Asr: "17:20", Maghrib: "20:55", Isha: "23:01"}
	// Just after Fajr, the next prayer is Dhuhr — not Sunrise.
	now, _ := time.Parse("15:04", "04:00")
	if name, _ := pt.Next(now); name != "Dhuhr" {
		t.Errorf("next after 04:00 = %q, want Dhuhr", name)
	}
	// After Isha it wraps to tomorrow's Fajr.
	late, _ := time.Parse("15:04", "23:30")
	if name, _ := pt.Next(late); name != "Fajr" {
		t.Errorf("next after 23:30 = %q, want Fajr", name)
	}
}
