package images

import (
	"testing"
	"time"
)

func TestDailyImageDue(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-03-29", "2026-10-25"} {
		day, err := time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			t.Fatal(err)
		}
		due := time.Date(day.Year(), day.Month(), day.Day(), 6, 0, 0, 0, loc)
		if imageDue(due.Add(-time.Second), Daily{}) {
			t.Fatal("generated before six")
		}
		if !imageDue(due, Daily{Date: "yesterday", URL: "https://example.com/old.png"}) {
			t.Fatal("missed daily image")
		}
		if imageDue(due.Add(time.Hour), Daily{Date: date, URL: "https://example.com/today.png"}) {
			t.Fatal("duplicate daily image")
		}
		if !imageDue(due, Daily{Date: date}) {
			t.Fatal("failed generation cannot retry")
		}
	}
}
