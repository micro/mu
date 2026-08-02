package events

// Standing instructions: an event that fires more than once, and one that runs
// the agent when it does.
//
// This is the thing Mu can do that almost no other MCP server can. Nearly all
// of them are a process a client starts and stops, so "every morning, brief me
// and mail it" has nowhere to live — there is no morning for a process that
// only exists while something is attached. Mu is a server that stays up, with
// a scheduler, an agent and an inbox already in it. The pieces were all here
// and nothing joined them.
//
// It stays inside events rather than becoming a second scheduler, because a
// repeating event is still a thing that happens at a time, which is what this
// service is for.

import (
	"fmt"
	"strings"
	"time"
)

// Repeat intervals. Anything else is treated as no repeat, so a typo leaves a
// one-off event rather than an unstoppable one.
const (
	RepeatNone    = ""
	RepeatHourly  = "hourly"
	RepeatDaily   = "daily"
	RepeatWeekly  = "weekly"
	RepeatMonthly = "monthly"
)

// ParseRepeat normalises a caller's word for how often something recurs.
func ParseRepeat(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hourly", "hour", "every hour":
		return RepeatHourly
	case "daily", "day", "every day", "each day":
		return RepeatDaily
	case "weekly", "week", "every week":
		return RepeatWeekly
	case "monthly", "month", "every month":
		return RepeatMonthly
	}
	return RepeatNone
}

// nextOccurrence is when a repeating event should next fire, counted from the
// time it was due rather than the time it actually ran. A daily briefing that
// fires late must not drift later every day.
func nextOccurrence(due time.Time, repeat string) (time.Time, bool) {
	switch repeat {
	case RepeatHourly:
		return due.Add(time.Hour), true
	case RepeatDaily:
		return due.AddDate(0, 0, 1), true
	case RepeatWeekly:
		return due.AddDate(0, 0, 7), true
	case RepeatMonthly:
		return due.AddDate(0, 1, 0), true
	}
	return time.Time{}, false
}

// catchUp moves an occurrence forward until it is in the future.
//
// An instance that was down for two days should not fire two days of missed
// briefings the moment it comes back, one after another, each of them stale.
// It should say the next one at the right time.
func catchUp(next time.Time, repeat string, now time.Time) (time.Time, bool) {
	for i := 0; !next.After(now); i++ {
		// A repeat that never advances would spin here; bail rather than hang.
		if i > 10000 {
			return time.Time{}, false
		}
		n, ok := nextOccurrence(next, repeat)
		if !ok {
			return time.Time{}, false
		}
		next = n
	}
	return next, true
}

// reschedule books the next occurrence of a repeating event. The caller holds
// the lock.
func rescheduleLocked(e *Event, now time.Time) {
	if e.Repeat == RepeatNone {
		return
	}
	next, ok := nextOccurrence(e.When, e.Repeat)
	if !ok {
		return
	}
	if next, ok = catchUp(next, e.Repeat, now); !ok {
		return
	}
	e.When = next
	e.Fired = false
	e.FiredAt = time.Time{}
}

// Describe says what a standing instruction does, in a sentence a person can
// check. An agent that has been told to do something every morning should be
// able to say so plainly.
func Describe(e *Event) string {
	when := e.When.Format("Mon 2 Jan 15:04 MST")
	if e.Repeat == RepeatNone {
		return fmt.Sprintf("%s at %s", e.Title, when)
	}
	return fmt.Sprintf("%s — %s, next at %s", e.Title, e.Repeat, when)
}
