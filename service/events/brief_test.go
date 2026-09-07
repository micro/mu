package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBriefScheduleLifecycle(t *testing.T) {
	reset()
	defer reset()
	for _, paused := range []bool{false, false, true, false} {
		if err := ScheduleBrief("alice", "20:00", "Europe/London", "daily", "evening", paused); err != nil {
			t.Fatal(err)
		}
		e := Brief("alice")
		if e == nil || e.Paused != paused || e.Prompt != "Give me a brief for tomorrow" {
			t.Fatalf("bad schedule: %+v", e)
		}
		if len(List("alice")) != 1 || Brief("bob") != nil {
			t.Fatal("duplicate or cross-account schedule")
		}
		if paused && len(Upcoming("alice")) != 0 {
			t.Fatal("paused event upcoming")
		}
	}
	if err := ScheduleBrief("bob", "07:00", "Asia/Tokyo", "weekdays", "morning", false); err != nil {
		t.Fatal(err)
	}
	if Brief("bob").Prompt != "Give me a brief for today" || Brief("alice").Zone != "Europe/London" {
		t.Fatal("owner isolation")
	}
}

func TestBriefScheduleValidation(t *testing.T) {
	reset()
	defer reset()
	cases := [][5]string{{"", "20:00", "Europe/London", "daily", "evening"}, {"a", "25:00", "Europe/London", "daily", "evening"}, {"a", "20:00", "invalid", "daily", "evening"}, {"a", "20:00", "Local", "daily", "evening"}, {"a", "20:00", "Europe/London", "bad", "evening"}, {"a", "20:00", "Europe/London", "daily", "bad"}}
	for _, c := range cases {
		if err := ScheduleBrief(c[0], c[1], c[2], c[3], c[4], false); err == nil {
			t.Fatalf("accepted %v", c)
		}
	}
	if len(events) != 0 {
		t.Fatal("invalid input wrote events")
	}
}

func TestBriefClockSurvivesRestartAndDST(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/London")
	for _, date := range []time.Time{time.Date(2026, 10, 24, 20, 0, 0, 0, loc), time.Date(2026, 3, 28, 20, 0, 0, 0, loc)} {
		original := Event{When: date, Zone: "Europe/London", Repeat: "daily"}
		raw, _ := json.Marshal(original)
		var e Event
		json.Unmarshal(raw, &e)
		rescheduleLocked(&e, date.Add(time.Minute))
		if e.When.In(loc).Hour() != 20 || e.When.Sub(date) == 24*time.Hour {
			t.Fatalf("DST drift: %v", e.When)
		}
	}
	friday := time.Date(2026, 9, 11, 20, 0, 0, 0, loc)
	next, ok := nextOccurrence(friday, "weekdays")
	if !ok || next.Weekday() != time.Monday || next.Hour() != 20 {
		t.Fatal(next)
	}
}

func TestPausedBriefDoesNotFire(t *testing.T) {
	reset()
	defer reset()
	events["paused"] = &Event{ID: "paused", Owner: "alice", Kind: "brief", Paused: true, When: time.Now().Add(-time.Hour), Repeat: "daily"}
	fired := false
	OnFire = func(_, _, _ string) { fired = true }
	fireDue()
	if fired || events["paused"].Fired {
		t.Fatal("paused brief fired")
	}
}
