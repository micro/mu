package events

import (
	"strings"
	"testing"
	"time"
)

// withExternal attaches a fake outside calendar for one test.
func withExternal(t *testing.T, connected bool, busy []Slot, entries []External) {
	t.Helper()
	pc, pb, pe, pa := ExternalConnected, ExternalBusy, ExternalEntries, ExternalAccount
	ExternalConnected = func(string) bool { return connected }
	ExternalBusy = func(string, time.Time, time.Time) []Slot { return busy }
	ExternalEntries = func(string, time.Time, time.Time) []External { return entries }
	ExternalAccount = func(string) string { return "someone@example.com" }
	t.Cleanup(func() {
		ExternalConnected, ExternalBusy, ExternalEntries, ExternalAccount = pc, pb, pe, pa
	})
}

// The whole point. "When am I free" was answered from the events Mu created,
// which is a fraction of anybody's week — so it offered slots that were already
// taken. Offering a Thursday that is actually full is worse than saying nothing.
func TestFreeCountsTheCalendarYouAlreadyKeep(t *testing.T) {
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	q := FreeQuery{From: day.Add(9 * time.Hour), To: day.Add(17 * time.Hour), Minutes: 60, DayStart: 9, DayEnd: 17}

	// Nothing attached: the whole working day is open.
	withExternal(t, false, nil, nil)
	if got := Free("nobody", q); len(got) != 1 || got[0].Duration() != 8*time.Hour {
		t.Fatalf("an empty calendar was not free all day: %+v", got)
	}

	// A meeting on the outside calendar splits it.
	withExternal(t, true, []Slot{{
		Start: day.Add(12 * time.Hour), End: day.Add(13 * time.Hour),
	}}, nil)

	slots := Free("nobody", q)
	if len(slots) != 2 {
		t.Fatalf("an outside meeting did not split the day: %+v", slots)
	}
	for _, s := range slots {
		if s.Start.Before(day.Add(13*time.Hour)) && s.End.After(day.Add(12*time.Hour)) {
			t.Errorf("a slot was offered over a booked hour: %v–%v", s.Start, s.End)
		}
	}
}

// Both calendars are one day. An event here and a meeting there overlap in the
// person's actual afternoon, so the busy periods must merge rather than each
// subtract on their own.
func TestBusyPeriodsFromBothCalendarsMerge(t *testing.T) {
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	owner := "merger"

	if _, err := CreateStanding(owner, "Standup", day.Add(12*time.Hour), "", 60, "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, e := range List(owner) {
			_ = Remove(owner, e.ID)
		}
	})

	// An outside meeting overlapping the second half of that hour.
	withExternal(t, true, []Slot{{
		Start: day.Add(12*time.Hour + 30*time.Minute), End: day.Add(14 * time.Hour),
	}}, nil)

	busy := booked(owner, day, day.AddDate(0, 0, 1))
	if len(busy) != 1 {
		t.Fatalf("overlapping busy periods did not merge: %+v", busy)
	}
	if !busy[0].Start.Equal(day.Add(12*time.Hour)) || !busy[0].End.Equal(day.Add(14*time.Hour)) {
		t.Errorf("merged period is %v–%v, want 12:00–14:00", busy[0].Start, busy[0].End)
	}
}

// The ask is placed where it is earned: at the moment someone gets a thinner
// answer than they wanted, not at signup.
func TestTheConnectHintAppearsOnlyWhenItWouldHelp(t *testing.T) {
	// Nothing wired at all (a self-hosted Mu with no Google credentials): never
	// dangle a button that cannot work.
	ExternalConnected = nil
	if got := connectHint("someone"); got != "" {
		t.Errorf("an instance with no client offered to connect one: %q", got)
	}

	// Wired but not connected: this is the one case that should ask.
	withExternal(t, false, nil, nil)
	if got := connectHint("someone"); !strings.Contains(got, "/events") {
		t.Errorf("someone with no calendar attached was not told they could: %q", got)
	}

	// Already connected: never nag.
	withExternal(t, true, nil, nil)
	if got := connectHint("someone"); got != "" {
		t.Errorf("someone who already connected was asked again: %q", got)
	}
}

// A day happens in one order. Two lists would have left the reader doing the
// merge, which is the work attaching a calendar was supposed to remove.
func TestBothCalendarsAreShownInTimeOrder(t *testing.T) {
	base := time.Now().Add(24 * time.Hour)
	mine := &Event{ID: "x", Title: "Mine", When: base.Add(2 * time.Hour)}
	theirs := External{Title: "Theirs", Start: base.Add(time.Hour), End: base.Add(90 * time.Minute)}

	rows := mergedRows([]*Event{mine}, []External{theirs})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Event != nil || rows[0].External.Title != "Theirs" {
		t.Errorf("the earlier entry is not first: %+v", rows)
	}
}

// Mu did not schedule an outside entry and cannot cancel it, so it must not
// render a control that would fail — and it must say where it came from.
func TestAnOutsideEntryOffersNothingItCannotDo(t *testing.T) {
	html := externalRow(External{
		Title: "Dentist", Start: time.Now(), End: time.Now().Add(time.Hour),
		Location: "Baker St", Source: "Google Calendar",
	})

	if strings.Contains(html, "action=\"cancel\"") || strings.Contains(html, "<form") {
		t.Errorf("an outside entry offered a cancel button:\n%s", html)
	}
	for _, want := range []string{"Dentist", "Baker St", "Google Calendar"} {
		if !strings.Contains(html, want) {
			t.Errorf("the row is missing %q:\n%s", want, html)
		}
	}
}

// The card is the consent ask, and it must not appear on an instance that
// cannot honour it.
func TestTheCalendarCardOnlyAppearsWhenItCanWork(t *testing.T) {
	ExternalConnected = nil
	if got := calendarCard("someone", "", "csrf-token"); got != "" {
		t.Errorf("an instance with no client rendered a connect card: %q", got)
	}

	withExternal(t, false, nil, nil)
	card := calendarCard("someone", "", "csrf-token")
	if !strings.Contains(card, "/oauth2/google/calendar") {
		t.Errorf("the card does not lead anywhere:\n%s", card)
	}
	if !strings.Contains(card, "read-only") && !strings.Contains(card, "Read-only") {
		t.Errorf("the card does not say how much access it wants:\n%s", card)
	}

	withExternal(t, true, nil, nil)
	connected := calendarCard("someone", "connected", "csrf-token")
	if !strings.Contains(connected, "someone@example.com") {
		t.Errorf("the card does not say which calendar is attached:\n%s", connected)
	}
	// Withdrawing access is one action covering everything granted — Google
	// revokes the whole grant at once — so it lives with the rest of the
	// inventory rather than being repeated on each page that uses a piece of
	// it. A per-page button would either lie about its scope or duplicate a
	// decision somebody should make while looking at the whole list.
	if strings.Contains(connected, "Disconnect") {
		t.Errorf("withdrawing access was duplicated onto the events page:\n%s", connected)
	}
	if !strings.Contains(connected, `href="/account"`) {
		t.Errorf("the card does not say where to manage it:\n%s", connected)
	}
}
