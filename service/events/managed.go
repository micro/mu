package events

import (
	"fmt"
	"mu/internal/data"
	"time"
)

// EditOwned atomically edits an owner's schedules without sending calendar
// invitations. The callback receives detached records and must not call Events.
// Feature owners use this for their private schedule state; it is not an RPC.
func EditOwned(owner string, edit func(map[string]*Event) error) error {
	if owner == "" {
		return fmt.Errorf("schedule owner required")
	}
	mu.Lock()
	defer mu.Unlock()
	owned := map[string]*Event{}
	for id, e := range events {
		if e.Owner == owner {
			cp := *e
			if e.WorldNews != nil {
				value := *e.WorldNews
				cp.WorldNews = &value
			}
			owned[id] = &cp
		}
	}
	if err := edit(owned); err != nil {
		return err
	}
	list := make([]*Event, 0, len(events)+len(owned))
	for _, e := range events {
		if e.Owner != owner {
			list = append(list, e)
		}
	}
	for id, e := range owned {
		if existing := events[id]; existing != nil && existing.Owner != owner {
			return fmt.Errorf("schedule id belongs to another owner")
		}
		if e == nil || e.ID != id || e.Owner != owner {
			return fmt.Errorf("invalid owned schedule")
		}
		list = append(list, e)
	}
	if err := data.SaveJSON(storeKey, list); err != nil {
		return err
	}
	for id, e := range events {
		if e.Owner == owner {
			delete(events, id)
		}
	}
	for id, e := range owned {
		cp := *e
		if e.WorldNews != nil {
			value := *e.WorldNews
			cp.WorldNews = &value
		}
		events[id] = &cp
	}
	return nil
}

// NextTime advances a recurring wall-clock schedule beyond now.
func NextTime(at time.Time, repeat string, now time.Time) (time.Time, bool) {
	return catchUp(at, repeat, now)
}
