package agent

import (
	"context"
	"fmt"
	"time"

	"mu/internal/auth"
	"mu/service/events"
)

// Included brief facts come from bounded owned records, never an open-ended
// tool loop. Planning shares the same model call and morning delivery.
func RunScheduled(owner, id, revision string, due time.Time) (handled bool, answer string, err error) {
	var schedule *events.Event
	for _, e := range events.List(owner) {
		if e.ID == id {
			schedule = e
			break
		}
	}
	if schedule == nil {
		return true, "", nil
	}
	if schedule.Kind != "brief" && schedule.Kind != "research" && schedule.Kind != "checkin" && schedule.Kind != "moment" {
		return false, "", nil
	}
	if schedule.Paused || fmt.Sprint(schedule.Sequence) != revision {
		return true, "", nil
	}
	acc, err := auth.GetAccount(owner)
	if err != nil || acc.Banned {
		return true, "", nil
	}
	now := time.Now()
	cadence := now
	if due.After(now) {
		cadence = due
	}
	allowed, reserveErr := reserveScheduled(schedule, due, cadence)
	if reserveErr != nil {
		return true, "", reserveErr
	}
	if !allowed {
		return true, "", nil
	}
	at := due
	if at.IsZero() {
		at = time.Now()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if schedule.Kind == "checkin" {
		answer = checkinMessage(owner, schedule, at)
	} else if schedule.Kind == "moment" {
		answer = momentMessage(schedule, at)
	} else if schedule.Kind == "research" {
		if auth.Plan(owner) != "pro" {
			return true, "", nil
		}
		answer, err = eveningDigest(schedule, at)
	} else {
		facts, reflection := morningFacts(ctx, owner, schedule, at)
		var content BriefContent
		content, err = IncludedBrief(ctx, owner, facts)
		if err == nil {
			answer = renderMorningBrief(content, acc.Name, schedule, at, reflection, auth.Plan(owner) == "pro" && schedule.Plan)
		}

	}
	return true, answer, err
}

// Agent owns feature cadence and entitlement; Events only advances the clock.
func reserveScheduled(source *events.Event, due, now time.Time) (bool, error) {
	allowed := false
	err := events.EditOwned(source.Owner, func(records map[string]*events.Event) error {
		e := records[source.ID]
		if e == nil || e.Paused || e.Sequence != source.Sequence {
			return nil
		}
		if retiredBrief(e) {
			delete(records, e.ID)
			return nil
		}
		if (e.Kind == "checkin" || e.Kind == "moment") && (due.IsZero() || now.Sub(due) > time.Hour) {
			return nil
		}
		if e.Kind == "research" && auth.Plan(e.Owner) != "pro" {
			e.Paused = true
			return nil
		}
		if e.Kind == "brief" && !briefEntitled(e.Owner) {
			return nil
		}
		if e.Kind == "brief" {
			e.Repeat = BriefFrequency(e.Owner, e.Repeat)
			loc, err := time.LoadLocation(e.Zone)
			if err != nil {
				loc = time.UTC
			}
			days := 1
			if e.Repeat == "weekly" {
				days = 7
			}
			if !e.LastBrief.IsZero() {
				earliest := e.LastBrief.In(loc).AddDate(0, 0, days)
				if now.Before(earliest) {
					if next, ok := events.NextTime(e.When.In(loc), e.Repeat, earliest.Add(-time.Nanosecond)); ok {
						e.When = next
					}
					return nil
				}
			}
			e.LastBrief = now
			if e.Repeat == "weekly" && !due.IsZero() && !e.When.Equal(due) {
				if next, ok := events.NextTime(due.In(loc), e.Repeat, now); ok {
					e.When = next
				}
			}
			source.Repeat = e.Repeat
		}
		allowed = true
		return nil
	})
	return allowed, err
}
