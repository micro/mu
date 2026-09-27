package events

import (
	"testing"
	"time"
)

func TestAdvancePreservesDeliveryAndRecurs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mu.Lock()
	previous := events
	events = map[string]*Event{}
	mu.Unlock()
	defer func() { mu.Lock(); events = previous; mu.Unlock() }()
	due := time.Now().UTC().Add(time.Hour)
	e, err := CreateScheduled("advance-owner", "Brief", due, "", 0, "daily", "Read news", Advance{Minutes: 10, Recipient: "agent"}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := Create("advance-owner", "Meeting", due, "")
	if err != nil {
		t.Fatal(err)
	}
	fireDueAt(due.Add(-11 * time.Minute))
	if !events[e.ID].AdvanceFor.IsZero() {
		t.Fatal("advanced too soon")
	}
	fireDueAt(due.Add(-10 * time.Minute))
	if !events[e.ID].AdvanceFor.Equal(due) || !events[e.ID].When.Equal(due) || events[e.ID].Fired {
		t.Fatal("preparation changed delivery")
	}
	if !events[ordinary.ID].AdvanceFor.IsZero() {
		t.Fatal("ordinary event prepared")
	}
	fireDueAt(due)
	next := due.AddDate(0, 0, 1)
	if !events[e.ID].When.Equal(next) {
		t.Fatal("recurrence lost")
	}
	fireDueAt(next.Add(-10 * time.Minute))
	if !events[e.ID].AdvanceFor.Equal(next) {
		t.Fatal("next occurrence not prepared")
	}
}

func TestAdvanceValidation(t *testing.T) {
	for _, a := range []Advance{{Minutes: -1, Recipient: "agent"}, {Minutes: 10, Recipient: "other"}, {Minutes: 10081, Recipient: "user"}} {
		if validAdvance(a, "Read news") == nil {
			t.Fatalf("accepted %+v", a)
		}
	}
	if validAdvance(Advance{Minutes: 10, Recipient: "agent"}, "") == nil {
		t.Fatal("agent trigger without instructions")
	}
}
