package events

import (
	"mu/internal/data"
	"mu/internal/group"
	"testing"
	"time"
)

func TestDeletedGroupSchedulesRetireBeforeFiring(t *testing.T) {
	g, err := group.Create("schedule-owner", "Schedules", false)
	if err != nil {
		t.Fatal(err)
	}
	owner := group.Scope(g.ID)
	due := time.Now().UTC().Add(time.Hour)
	for _, title := range []string{"Reminder", "Brief"} {
		if _, err := CreateScheduled(owner, title, due, "", 0, "daily", "Shared instruction", Advance{Minutes: 10, Recipient: "agent"}, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	personal, err := Create(g.Owner, "Personal", due, "")
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteAll(g.Owner)
	if err := group.Change(g.ID, g.Owner, "delete", "", ""); err != nil {
		t.Fatal(err)
	}
	// The first tick also handles records restored after a restart, before
	// publishing advance work, the reminder itself, or its next occurrence.
	fireDueAt(due.Add(-10 * time.Minute))
	if len(List(owner)) != 0 {
		t.Fatal("orphaned schedules survived")
	}
	if len(List(g.Owner)) != 1 || events[personal.ID].Fired {
		t.Fatal("changed personal schedule")
	}
	var saved []*Event
	if err := data.LoadJSON(storeKey, &saved); err != nil {
		t.Fatal(err)
	}
	for _, e := range saved {
		if e.Owner == owner {
			t.Fatal("orphan cleanup was not persisted")
		}
	}
	fireDueAt(due.Add(24 * time.Hour))
	if len(List(owner)) != 0 {
		t.Fatal("orphaned recurrence recreated")
	}
}
