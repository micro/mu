package work

import (
	"mu/service/events"
	"strings"
	"testing"
	"time"
)

func TestCheckinUsesOnlyOwnedTodayContext(t *testing.T) {
	owner, foreign := "checkin-context-owner", "checkin-context-foreign"
	defer events.DeleteAll(owner)
	defer events.DeleteAll(foreign)
	now := time.Now().UTC()
	// Start tomorrow to make a same-day window independent of the test's hour.
	now = time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, time.UTC)
	if _, err := events.Create(owner, "Proposal meeting", now.Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create(foreign, "Private foreign appointment", now.Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create(owner, "Tomorrow appointment", now.AddDate(0, 0, 1), ""); err != nil {
		t.Fatal(err)
	}
	message := checkinMessage(owner, &events.Event{Zone: "UTC"}, now)
	if !strings.Contains(message, "Proposal meeting") || !strings.Contains(message, "One or two sentences") {
		t.Fatal(message)
	}
	if strings.Contains(message, "Private foreign") || strings.Contains(message, "Tomorrow appointment") {
		t.Fatal("unrelated context included")
	}
}
