package images

import (
	"testing"
	"time"
)

func TestConfiguredImageTimezone(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	now := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC).In(imageLocation())
	if now.Hour() != 5 || imageTime(now).UTC().Hour() != 10 {
		t.Fatal(now, imageTime(now))
	}
	t.Setenv("TZ", "invalid")
	if imageLocation() != time.UTC {
		t.Fatal("invalid zone must default to UTC")
	}
}
