package agent

import (
	"mu/service/events"
	"testing"
	"time"
)

func TestScheduledCheckinSkipsMissedAndEditedOccurrences(t *testing.T) {
	const owner = "checkin-late-owner"
	defer events.DeleteAll(owner)
	if err := scheduleCheckin(owner, "09:00", "Europe/London", "daily", false); err != nil {
		t.Fatal(err)
	}
	e := Checkin(owner)
	now := time.Now()
	if allowed, err := reserveScheduled(e, now.Add(-2*time.Hour), now); err != nil || allowed {
		t.Fatalf("late checkin: %v", err)
	}
	if allowed, err := reserveScheduled(e, now.Add(-time.Minute), now); err != nil || !allowed {
		t.Fatalf("timely checkin: %v", err)
	}
	if err := scheduleCheckin(owner, "10:00", "Europe/London", "daily", false); err != nil {
		t.Fatal(err)
	}
	if allowed, err := reserveScheduled(e, now, now); err != nil || allowed {
		t.Fatalf("stale revision: %v", err)
	}
}
