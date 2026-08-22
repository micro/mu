// Package events is Mu's personal scheduling service. A user asks — via the
// agent ("remind me to call the dentist at 3pm") or the /events page — and Mu
// stores the event and fires it at the appointed time, delivering the reminder
// over the user's linked channels (mail, the web) via the OnFire
// hook. "Schedule a reminder" and "create an event" are the same thing here.
package events

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/service"
)

// Event is a scheduled reminder owned by a single user.
type Event struct {
	ID    string    `json:"id"`
	Owner string    `json:"owner"`
	Title string    `json:"title"`
	When  time.Time `json:"when"`
	Note  string    `json:"note,omitempty"`
	// Minutes is how long the event lasts. Zero means the half hour the .ics
	// export has always assumed, so events stored before this existed keep the
	// meaning they were saved with.
	Minutes int `json:"minutes,omitempty"`
	// Repeat makes this a standing instruction rather than a one-off: see
	// repeat.go. Empty fires once.
	Repeat string `json:"repeat,omitempty"`
	// Prompt turns firing into work. When set, the agent runs it and delivers
	// the answer — which is what makes "every morning, brief me" possible on a
	// server that stays up.
	Prompt  string    `json:"prompt,omitempty"`
	Created time.Time `json:"created"`
	Fired   bool      `json:"fired"`
	FiredAt time.Time `json:"fired_at,omitempty"`
}

const storeKey = "events.json"

// retention is how long fired events are kept before being pruned.
const retention = 30 * 24 * time.Hour

var (
	mu     sync.RWMutex
	events = map[string]*Event{}
)

// OnFire is called when an event comes due. main.go sets it to deliver the
// reminder over the user's linked channels; kept as a hook so this package does
// not import the client packages (and to avoid an import cycle).
var OnFire func(accountID, title, note string)

// OnCreate is called when an event is scheduled. main.go sets it to email the
// owner an .ics calendar invite (if they have a verified email — e.g. from
// Google sign-in), so the event also lands in their real calendar. Kept as a
// hook so this package doesn't import mail/auth.
var OnCreate func(e *Event)

// Load reads persisted events, registers the go-micro service (which makes
// Create/List available to the agent, MCP and REST), and starts the scheduler.
func Load() {
	mu.Lock()
	var list []*Event
	if err := data.LoadJSON(storeKey, &list); err == nil {
		for _, e := range list {
			if e != nil && e.ID != "" {
				events[e.ID] = e
			}
		}
	}
	mu.Unlock()

	if err := service.Register(Spec); err != nil {
		app.Log("events", "service register failed: %v", err)
	}
	go scheduler()
}

// saveLocked persists the store. Callers must hold mu.
func saveLocked() {
	list := make([]*Event, 0, len(events))
	for _, e := range events {
		list = append(list, e)
	}
	if err := data.SaveJSON(storeKey, list); err != nil {
		app.Log("events", "save failed: %v", err)
	}
}

// Create schedules an event for owner at the given time.
func Create(owner, title string, when time.Time, note string) (*Event, error) {
	return CreateFor(owner, title, when, note, 0)
}

// CreateFor schedules an event of a given length in minutes. Zero takes the
// default.
func CreateFor(owner, title string, when time.Time, note string, minutes int) (*Event, error) {
	return CreateStanding(owner, title, when, note, minutes, "", "")
}

// CreateStanding schedules an event that may repeat, and may run a prompt
// through the agent when it fires.
func CreateStanding(owner, title string, when time.Time, note string, minutes int, repeat, prompt string) (*Event, error) {
	owner = strings.TrimSpace(owner)
	title = strings.TrimSpace(title)
	if owner == "" {
		return nil, fmt.Errorf("sign in to schedule events")
	}
	if title == "" {
		return nil, fmt.Errorf("a title is required")
	}
	if when.IsZero() {
		return nil, fmt.Errorf("a valid time is required")
	}
	e := &Event{
		ID:      uuid.New().String(),
		Owner:   owner,
		Title:   title,
		When:    when.UTC(),
		Note:    strings.TrimSpace(note),
		Minutes: minutes,
		Repeat:  ParseRepeat(repeat),
		Prompt:  strings.TrimSpace(prompt),
		Created: time.Now().UTC(),
	}
	mu.Lock()
	events[e.ID] = e
	saveLocked()
	mu.Unlock()

	if OnCreate != nil {
		cp := *e
		go OnCreate(&cp)
	}
	return e, nil
}

// List returns owner's events, soonest first.
func List(owner string) []*Event {
	mu.RLock()
	defer mu.RUnlock()
	var out []*Event
	for _, e := range events {
		if e.Owner == owner {
			cp := *e
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.Before(out[j].When) })
	return out
}

// Upcoming returns owner's not-yet-fired events, soonest first.
func Upcoming(owner string) []*Event {
	var out []*Event
	for _, e := range List(owner) {
		if !e.Fired {
			out = append(out, e)
		}
	}
	return out
}

// Cancel removes an event the owner owns.
func Cancel(owner, id string) error {
	mu.Lock()
	defer mu.Unlock()
	e := events[id]
	if e == nil || e.Owner != owner {
		return fmt.Errorf("event not found")
	}
	delete(events, id)
	saveLocked()
	return nil
}

func scheduler() {
	// A short cadence so reminders land close to their minute without a busy
	// loop. Runs once immediately, then every 30s.
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		fireDue()
		<-t.C
	}
}

// fireDue marks every due event fired (under lock, persisting once) then
// delivers them outside the lock so a slow channel can't block the store.
func fireDue() {
	now := time.Now().UTC()
	var due []*Event

	mu.Lock()
	changed := false
	for id, e := range events {
		if !e.Fired && !e.When.After(now) {
			e.Fired = true
			e.FiredAt = now
			cp := *e
			due = append(due, &cp)
			changed = true
			// A standing instruction books its next occurrence immediately, so
			// the schedule survives a restart between firings.
			rescheduleLocked(e, now)
		}
		// Prune long-fired events to keep the store bounded.
		if e.Fired && now.Sub(e.FiredAt) > retention {
			delete(events, id)
			changed = true
		}
	}
	if changed {
		saveLocked()
	}
	mu.Unlock()

	for _, e := range due {
		if OnFire != nil {
			OnFire(e.Owner, e.Title, e.Note)
		}
		// And the work, where the event carries an instruction. Announced
		// rather than called: see run.go. OnFire stays as it was, so being
		// told a reminder fired does not depend on any of this.
		requestWork(e)
	}
}

// Remove cancels an event the caller owns.
//
// Scheduling something you cannot unschedule is not safe to use for anything
// real: an agent could set a standing instruction to run every morning and
// nobody — including the person paying for each run — had a way to stop it.
// Create, Free and List were the whole surface.
func Remove(owner, id string) error {
	owner = strings.TrimSpace(owner)
	id = strings.TrimSpace(id)
	if owner == "" {
		return fmt.Errorf("sign in to change events")
	}
	if id == "" {
		return fmt.Errorf("which event?")
	}

	mu.Lock()
	defer mu.Unlock()

	e, ok := events[id]
	// Owner checked inside the lock and reported the same way as a missing one,
	// so this cannot be used to find out whether an id exists.
	if !ok || e.Owner != owner {
		return fmt.Errorf("no such event")
	}
	delete(events, id)
	saveLocked()
	return nil
}

// DeleteAll removes every event an owner has scheduled.
//
// Called when the account is deleted (internal/server/hooks.go). Events lived
// on after their owner: the scheduler would keep firing a standing instruction
// for an account that no longer exists, and a deleted person's calendar stayed
// on disk.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	removed := 0
	for id, e := range events {
		if e != nil && e.Owner == owner {
			delete(events, id)
			removed++
		}
	}
	if removed > 0 {
		saveLocked()
		app.Log("events", "deleted %d events for %s", removed, owner)
	}
}
