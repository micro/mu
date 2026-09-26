package events

import (
	"mu/internal/data"
	"testing"
	"time"
)

func TestCheckinIndependentAndDurable(t *testing.T) {
	const owner = "checkin_schedule_test"
	defer DeleteAll(owner)
	if Checkin(owner) != nil {
		t.Fatal("automatically enrolled")
	}
	if err := ScheduleBrief(owner, "06:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	brief := Brief(owner)
	if err := scheduleCheckin(owner, "09:00", "Europe/London", "weekdays", false); err != nil {
		t.Fatal(err)
	}
	first := Checkin(owner)
	if first == nil || first.When.In(mustLondon(t)).Hour() != 9 {
		t.Fatal("wrong default schedule")
	}
	if err := scheduleCheckin(owner, "10:00", "Europe/London", "weekdays", true); err != nil {
		t.Fatal(err)
	}
	updated := Checkin(owner)
	if updated.ID != first.ID || !updated.Paused || updated.When.In(mustLondon(t)).Hour() != 10 {
		t.Fatal("update lost identity or settings")
	}
	if got := Brief(owner); got.ID != brief.ID || !got.When.Equal(brief.When) || got.Paused {
		t.Fatal("brief changed")
	}
	if Checkin("another-owner") != nil {
		t.Fatal("account isolation")
	}
	if err := scheduleCheckin(owner, "bad", "Europe/London", "daily", false); err == nil {
		t.Fatal("invalid time accepted")
	}
	var persisted []*Event
	if err := data.LoadJSON(storeKey, &persisted); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range persisted {
		if e.ID == first.ID {
			found = e.Paused && e.Repeat == "weekdays"
		}
	}
	if !found {
		t.Fatal("settings not persisted")
	}
	// Recurrence stays at the local hour when UK daylight saving ends.
	due := time.Date(2026, 10, 24, 9, 0, 0, 0, mustLondon(t))
	next, ok := nextOccurrence(due, "daily")
	if !ok || next.Hour() != 9 || next.Sub(due) != 25*time.Hour {
		t.Fatal("DST moved local time")
	}
}
func mustLondon(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
