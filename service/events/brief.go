package events

import (
	"fmt"
	"github.com/google/uuid"
	"mu/internal/data"
	"time"
	_ "time/tzdata"
)

// Brief returns the owner's Home brief schedule, including a paused one.
func Brief(owner string) *Event {
	for _, e := range List(owner) {
		if e.Kind == "brief" {
			return e
		}
	}
	return nil
}

// ScheduleBrief updates one standing instruction atomically, without creating
// calendar invitations. Repeated form submissions cannot duplicate the job.
func ScheduleBrief(owner, clock, zone, repeat, period string, paused bool) error {
	return scheduleBrief(owner, clock, zone, repeat, period, paused, false)
}

func scheduleBrief(owner, clock, zone, repeat, period string, paused, builtin bool, news ...bool) error {
	if owner == "" {
		return fmt.Errorf("sign in to schedule a brief")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		return fmt.Errorf("choose a valid timezone")
	}
	t, err := time.Parse("15:04", clock)
	if err != nil {
		return fmt.Errorf("choose a valid time")
	}
	if repeat != "daily" && repeat != "weekdays" {
		return fmt.Errorf("choose daily or weekdays")
	}
	prompt := "Give me a brief for tomorrow"
	if period == "morning" {
		prompt = "Give me a brief for today"
	} else if period != "evening" {
		return fmt.Errorf("choose morning or evening")
	}
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	for !next.After(now) || (repeat == "weekdays" && (next.Weekday() == time.Saturday || next.Weekday() == time.Sunday)) {
		next = next.AddDate(0, 0, 1)
	}
	mu.Lock()
	defer mu.Unlock()
	var old *Event
	for _, e := range events {
		if e.Owner == owner && e.Kind == "brief" {
			old = e
			break
		}
	}
	if builtin && old != nil && !legacyBrief(old) {
		return nil
	}
	e := &Event{ID: uuid.New().String(), Owner: owner, Created: time.Now().UTC()}
	if old != nil {
		*e = *old
	}
	e.Kind, e.Title, e.When, e.Zone, e.Repeat, e.Prompt, e.Paused = "brief", "Evening brief", next, zone, repeat, prompt, paused
	e.Fired, e.FiredAt = false, time.Time{}
	e.Builtin = builtin
	if builtin && old != nil {
		e.Paused = old.Paused
	}
	if period == "morning" {
		e.Title = "Morning brief"
	}
	if len(news) > 0 {
		value := news[0]
		e.WorldNews = &value
	}
	e.Sequence++
	events[e.ID] = e
	list := make([]*Event, 0, len(events))
	for _, v := range events {
		list = append(list, v)
	}
	if err := data.SaveJSON(storeKey, list); err != nil {
		if old == nil {
			delete(events, e.ID)
		} else {
			events[e.ID] = old
		}
		return err
	}
	return nil
}

// Existing daily briefs remain intact. New accounts are not enrolled implicitly.
func ensureDefaultBriefs() {}

// The first shipped brief default was a tomorrow brief at 20:00. Migrate that
// exact legacy preset once; renamed schedules and other times stay untouched.
func legacyBrief(e *Event) bool {
	if e == nil || e.Kind != "brief" || e.Title != "Daily brief" || e.Prompt != "Give me a brief for tomorrow" || e.Repeat != "daily" {
		return false
	}
	loc, err := time.LoadLocation(e.Zone)
	if err != nil {
		return false
	}
	at := e.When.In(loc)
	return at.Hour() == 20 && at.Minute() == 0
}

// BriefWorldNews preserves the existing inclusion for schedules made before this preference.
func BriefWorldNews(e *Event) bool { return e == nil || e.WorldNews == nil || *e.WorldNews }

// ConfigureBrief changes one owned schedule atomically. Toggles preserve its identity and cadence.
func ConfigureBrief(owner string, enabled, news bool, zone string) error {
	if owner == "" {
		return fmt.Errorf("sign in to change your brief")
	}
	mu.Lock()
	defer mu.Unlock()
	var old *Event
	for _, e := range events {
		if e.Owner == owner && e.Kind == "brief" {
			old = e
			break
		}
	}
	var e Event
	if old != nil {
		e = *old
	} else {
		loc, err := time.LoadLocation(zone)
		if err != nil || zone == "" || zone == "Local" {
			return fmt.Errorf("a valid timezone is needed for your morning brief")
		}
		now := time.Now().In(loc)
		e = Event{ID: uuid.NewString(), Owner: owner, Kind: "brief", Builtin: true, Title: "Morning brief", Zone: zone, Repeat: "daily", Prompt: "Give me a brief for today", Created: time.Now().UTC(), When: time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, loc)}
	}
	e.Builtin = false // An explicit preference, not automatic enrollment.
	e.Paused = !enabled
	e.WorldNews = &news
	e.Sequence++
	if enabled && !e.When.After(time.Now()) {
		at := e.When
		if loc, err := time.LoadLocation(e.Zone); err == nil {
			at = at.In(loc)
		}
		next, ok := catchUp(at, e.Repeat, time.Now())
		if !ok {
			return fmt.Errorf("invalid brief schedule")
		}
		e.When = next
	}
	e.Fired = false
	e.FiredAt = time.Time{}
	list := make([]*Event, 0, len(events)+1)
	for id, v := range events {
		if id != e.ID {
			list = append(list, v)
		}
	}
	list = append(list, &e)
	if err := data.SaveJSON(storeKey, list); err != nil {
		return err
	}
	events[e.ID] = &e
	return nil
}
