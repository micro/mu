package images

import (
	"testing"
	"time"
)

func TestDailyImageWaitsForSixIncludingRestart(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/London")
	for _, hour := range []int{0, 5, 6, 12, 23} {
		now := time.Date(2026, 9, 8, hour, 0, 0, 0, loc)
		if got := imageDue(now, Daily{}); got != (hour >= 6) {
			t.Fatalf("hour %d due=%v", hour, got)
		}
		if imageDue(now, Daily{Date: "2026-09-08", URL: "already-generated"}) {
			t.Fatal("duplicate image")
		}
		if got := imageDue(now, Daily{Date: "2026-09-07", URL: "yesterday"}); got != (hour >= 6) {
			t.Fatal("yesterday image timing")
		}
	}
	for _, now := range []time.Time{time.Date(2026, 3, 28, 12, 0, 0, 0, loc), time.Date(2026, 10, 24, 12, 0, 0, 0, loc)} {
		next := imageTime(now).AddDate(0, 0, 1)
		if next.Hour() != 6 || next.Sub(imageTime(now)) == 24*time.Hour {
			t.Fatalf("DST drift: %v", next)
		}
	}
}
