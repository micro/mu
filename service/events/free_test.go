package events

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-events-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// day builds a time on a fixed date, so these tests do not drift with the clock.
func day(hour, min int) time.Time {
	return time.Date(2026, 8, 3, hour, min, 0, 0, time.UTC)
}

func clear(owner string) {
	for _, e := range List(owner) {
		_ = Cancel(owner, e.ID)
	}
}

// An empty calendar is free for the whole working day, not for the whole day:
// offering 3am is not an answer anybody wants.
func TestFreeRespectsWorkingHours(t *testing.T) {
	clear("alice")
	slots := Free("alice", FreeQuery{
		From: day(0, 0), To: day(23, 59), Minutes: 30, DayStart: 9, DayEnd: 18,
	})
	if len(slots) != 1 {
		t.Fatalf("expected one open stretch, got %d: %v", len(slots), slots)
	}
	if slots[0].Start.Hour() != 9 || slots[0].End.Hour() != 18 {
		t.Errorf("working hours were not applied: %s–%s", slots[0].Start, slots[0].End)
	}
}

// A booked hour is subtracted, leaving the day either side of it.
func TestFreeSubtractsWhatIsBooked(t *testing.T) {
	clear("alice")
	if _, err := CreateFor("alice", "Standup", day(11, 0), "", 60); err != nil {
		t.Fatal(err)
	}
	defer clear("alice")

	slots := Free("alice", FreeQuery{From: day(0, 0), To: day(23, 59), Minutes: 30, DayStart: 9, DayEnd: 18})
	if len(slots) != 2 {
		t.Fatalf("expected the day either side of the meeting, got %v", slots)
	}
	if slots[0].End != day(11, 0) {
		t.Errorf("the morning does not end when the meeting starts: %s", slots[0].End)
	}
	if slots[1].Start != day(12, 0) {
		t.Errorf("the afternoon does not start when the meeting ends: %s", slots[1].Start)
	}
}

// The length asked for is the whole point: a 15-minute gap is not an hour.
func TestFreeOnlyOffersSlotsLongEnough(t *testing.T) {
	clear("alice")
	// Leave exactly 30 minutes free between two meetings.
	if _, err := CreateFor("alice", "One", day(9, 0), "", 120); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFor("alice", "Two", day(11, 30), "", 390); err != nil {
		t.Fatal(err)
	}
	defer clear("alice")

	if got := Free("alice", FreeQuery{From: day(0, 0), To: day(23, 59), Minutes: 30, DayStart: 9, DayEnd: 18}); len(got) != 1 {
		t.Errorf("the 30-minute gap was not offered for a 30-minute slot: %v", got)
	}
	if got := Free("alice", FreeQuery{From: day(0, 0), To: day(23, 59), Minutes: 60, DayStart: 9, DayEnd: 18}); len(got) != 0 {
		t.Errorf("a 30-minute gap was offered for an hour: %v", got)
	}
}

// Overlapping events must not each subtract separately, or the calendar reports
// gaps that are not there.
func TestOverlappingEventsAreMerged(t *testing.T) {
	clear("alice")
	if _, err := CreateFor("alice", "Long", day(10, 0), "", 180); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFor("alice", "Inside it", day(11, 0), "", 30); err != nil {
		t.Fatal(err)
	}
	defer clear("alice")

	slots := Free("alice", FreeQuery{From: day(0, 0), To: day(23, 59), Minutes: 30, DayStart: 9, DayEnd: 18})
	for _, s := range slots {
		if s.Start.Before(day(13, 0)) && s.End.After(day(10, 0)) {
			t.Errorf("a slot was offered inside a booked stretch: %s–%s", s.Start, s.End)
		}
	}
}

// One person's calendar is not another's.
func TestFreeIsPerOwner(t *testing.T) {
	clear("alice")
	clear("bob")
	if _, err := CreateFor("alice", "Alice only", day(9, 0), "", 540); err != nil {
		t.Fatal(err)
	}
	defer clear("alice")

	if got := Free("alice", FreeQuery{From: day(0, 0), To: day(23, 59), DayStart: 9, DayEnd: 18}); len(got) != 0 {
		t.Errorf("alice's day is full but she was offered %v", got)
	}
	if got := Free("bob", FreeQuery{From: day(0, 0), To: day(23, 59), DayStart: 9, DayEnd: 18}); len(got) == 0 {
		t.Error("bob was blocked by alice's calendar")
	}
}

// An event stored before durations existed still means half an hour, which is
// what the .ics export has always claimed for it.
func TestEventsWithoutADurationKeepTheirOldMeaning(t *testing.T) {
	e := &Event{Title: "Old"}
	if e.Length() != defaultDuration {
		t.Errorf("an event with no duration is %s, want %s", e.Length(), defaultDuration)
	}
	withLength := &Event{Title: "New", Minutes: 45}
	if withLength.Length() != 45*time.Minute {
		t.Errorf("a 45 minute event is %s", withLength.Length())
	}
}

func TestRenderSlotsReadsLikeACalendar(t *testing.T) {
	got := RenderSlots([]Slot{{Start: day(9, 0), End: day(11, 0)}}, 60)
	for _, want := range []string{"Mon 3 Aug", "09:00–11:00", "2 hr"} {
		if !contains(got, want) {
			t.Errorf("rendered slots missing %q:\n%s", want, got)
		}
	}
	if got := RenderSlots(nil, 30); got == "" {
		t.Error("an empty calendar rendered nothing at all")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
