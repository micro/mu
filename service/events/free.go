package events

// Finding a free slot.
//
// events could schedule a thing and list what was coming, which makes it a
// reminder queue. The question a person actually asks a calendar is the other
// one — "when am I free on Thursday?" — and answering it needs two facts this
// package did not have: how long an event lasts, and what counts as a working
// hour.
//
// Duration is now on the event (defaulting to the same 30 minutes the .ics
// export always assumed, so nothing already stored changes meaning). Working
// hours are a property of the search, not of the account, because the honest
// answer to "when am I free" depends on what for: a 9am call and a 9pm one have
// different windows and neither is a setting.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Slot is a stretch of unbooked time.
type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Duration is how long the slot is.
func (s Slot) Duration() time.Duration { return s.End.Sub(s.Start) }

// FreeQuery asks when an owner is free.
type FreeQuery struct {
	From     time.Time
	To       time.Time
	Minutes  int // the length of slot being looked for
	DayStart int // first hour of the day to consider, local to From's zone
	DayEnd   int // last hour, exclusive
}

// Free returns the stretches inside the window where nothing is booked and the
// slot is at least Minutes long.
func Free(owner string, q FreeQuery) []Slot {
	if q.Minutes <= 0 {
		q.Minutes = 30
	}
	if q.DayEnd <= q.DayStart {
		q.DayStart, q.DayEnd = 9, 18
	}
	if q.To.Before(q.From) {
		q.To = q.From.Add(7 * 24 * time.Hour)
	}
	want := time.Duration(q.Minutes) * time.Minute

	busy := booked(owner, q.From, q.To)

	var out []Slot
	// Walk a day at a time so working hours can be applied per day rather than
	// across the whole window, which is what makes "Thursday afternoon" mean
	// anything.
	for day := startOfDay(q.From); day.Before(q.To); day = day.AddDate(0, 0, 1) {
		open := day.Add(time.Duration(q.DayStart) * time.Hour)
		close := day.Add(time.Duration(q.DayEnd) * time.Hour)
		if open.Before(q.From) {
			open = q.From
		}
		if close.After(q.To) {
			close = q.To
		}
		if !open.Before(close) {
			continue
		}
		out = append(out, freeWithin(open, close, busy, want)...)
	}
	return out
}

// freeWithin subtracts the busy periods from one open stretch.
func freeWithin(open, close time.Time, busy []Slot, want time.Duration) []Slot {
	var out []Slot
	cursor := open
	for _, b := range busy {
		if !b.End.After(cursor) || !b.Start.Before(close) {
			continue // entirely before or after this stretch
		}
		if b.Start.After(cursor) && b.Start.Sub(cursor) >= want {
			out = append(out, Slot{Start: cursor, End: b.Start})
		}
		if b.End.After(cursor) {
			cursor = b.End
		}
	}
	if close.Sub(cursor) >= want {
		out = append(out, Slot{Start: cursor, End: close})
	}
	return out
}

// booked is the owner's events in the window as busy periods, merged so
// overlapping events do not each subtract separately.
//
// Both calendars, when a second one is attached. Merging is what makes the
// answer usable: a meeting that exists in Google and a reminder that exists
// here overlap in the person's actual day, and two lists would have made the
// caller reconcile them.
func booked(owner string, from, to time.Time) []Slot {
	var busy []Slot
	for _, e := range List(owner) {
		start := e.When
		end := start.Add(e.Length())
		if !end.After(from) || !start.Before(to) {
			continue
		}
		busy = append(busy, Slot{Start: start, End: end})
	}
	for _, s := range externalBusy(owner, from, to) {
		if !s.End.After(from) || !s.Start.Before(to) {
			continue
		}
		busy = append(busy, s)
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].Start.Before(busy[j].Start) })

	var merged []Slot
	for _, b := range busy {
		if n := len(merged); n > 0 && !b.Start.After(merged[n-1].End) {
			if b.End.After(merged[n-1].End) {
				merged[n-1].End = b.End
			}
			continue
		}
		merged = append(merged, b)
	}
	return merged
}

// Length is how long an event lasts, defaulting to the same half hour the .ics
// export has always assumed for events stored without one.
func (e *Event) Length() time.Duration {
	if e.Minutes > 0 {
		return time.Duration(e.Minutes) * time.Minute
	}
	return defaultDuration
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// RenderSlots writes slots the way a person reads them, grouped by day.
func RenderSlots(slots []Slot, want int) string {
	if len(slots) == 0 {
		return "No free slots in that window."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Free for at least %d minutes:\n", want)
	day := ""
	for _, s := range slots {
		if d := s.Start.Format("Mon 2 Jan"); d != day {
			day = d
			fmt.Fprintf(&b, "\n%s\n", d)
		}
		fmt.Fprintf(&b, "  %s–%s (%s)\n",
			s.Start.Format("15:04"), s.End.Format("15:04"), humanMinutes(s.Duration()))
	}
	return strings.TrimSpace(b.String())
}

func humanMinutes(d time.Duration) string {
	mins := int(d.Minutes())
	if mins < 60 {
		return fmt.Sprintf("%d min", mins)
	}
	h, m := mins/60, mins%60
	if m == 0 {
		return fmt.Sprintf("%d hr", h)
	}
	return fmt.Sprintf("%d hr %d min", h, m)
}
