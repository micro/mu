package events

import (
	"testing"
	"time"
)

func TestParseRepeat(t *testing.T) {
	for word, want := range map[string]string{
		"daily": RepeatDaily, "Daily": RepeatDaily, "every day": RepeatDaily,
		"hourly": RepeatHourly, "weekly": RepeatWeekly, "monthly": RepeatMonthly,
		"": RepeatNone, "fortnightly": RepeatNone, "nonsense": RepeatNone,
	} {
		if got := ParseRepeat(word); got != want {
			t.Errorf("ParseRepeat(%q) = %q, want %q", word, got, want)
		}
	}
}

// A typo must leave a one-off event rather than an unstoppable one.
func TestAnUnknownRepeatFiresOnce(t *testing.T) {
	e, err := CreateStanding("alice", "Once", time.Now().Add(time.Hour), "", 0, "fortnightly", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel("alice", e.ID)
	if e.Repeat != RepeatNone {
		t.Errorf("an unrecognised repeat became %q", e.Repeat)
	}
}

// The next occurrence is counted from when it was due, not when it ran, or a
// daily briefing drifts later every day.
func TestRepeatsDoNotDrift(t *testing.T) {
	due := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	// It actually fired 4 minutes late.
	ranAt := due.Add(4 * time.Minute)

	e := &Event{When: due, Repeat: RepeatDaily}
	rescheduleLocked(e, ranAt)

	want := time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC)
	if !e.When.Equal(want) {
		t.Errorf("next occurrence is %s, want %s — a late run moved the schedule", e.When, want)
	}
	if e.Fired {
		t.Error("a rescheduled event is still marked fired, so it will never fire again")
	}
}

// An instance that was down for two days must not fire two days of missed
// briefings the moment it returns, each of them stale.
func TestMissedOccurrencesDoNotStackUp(t *testing.T) {
	due := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)
	backUp := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

	e := &Event{When: due, Repeat: RepeatDaily}
	rescheduleLocked(e, backUp)

	if !e.When.After(backUp) {
		t.Errorf("next occurrence %s is not in the future (now %s)", e.When, backUp)
	}
	want := time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC)
	if !e.When.Equal(want) {
		t.Errorf("next occurrence is %s, want the next real one at %s", e.When, want)
	}
}

// A one-off stays fired.
func TestNonRepeatingEventsAreNotRescheduled(t *testing.T) {
	due := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	e := &Event{When: due, Fired: true, FiredAt: due}
	rescheduleLocked(e, due.Add(time.Minute))
	if !e.When.Equal(due) || !e.Fired {
		t.Error("a one-off event was rescheduled")
	}
}

func TestCatchUpTerminates(t *testing.T) {
	// A repeat that cannot advance must bail rather than spin.
	if _, ok := catchUp(time.Now().Add(-time.Hour), "not-a-repeat", time.Now()); ok {
		t.Error("an unadvanceable repeat reported a next occurrence")
	}
}

// A standing instruction carries the work to do, and says so plainly.
func TestStandingInstructionCarriesItsPrompt(t *testing.T) {
	when := time.Now().Add(time.Hour)
	e, err := CreateStanding("alice", "Morning brief", when, "", 0, "daily", "brief me on today's news")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel("alice", e.ID)

	if e.Prompt != "brief me on today's news" {
		t.Errorf("the prompt was lost: %q", e.Prompt)
	}
	if e.Repeat != RepeatDaily {
		t.Errorf("repeat is %q, want daily", e.Repeat)
	}
	if got := Describe(e); got == "" || !contains(got, "daily") {
		t.Errorf("a standing instruction does not describe itself: %q", got)
	}
}
