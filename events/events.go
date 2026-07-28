// Package events is Mu's personal scheduling service. A user asks — via the
// agent ("remind me to call the dentist at 3pm") or the /events page — and Mu
// stores the event and fires it at the appointed time, delivering the reminder
// over the user's linked channels (Discord, Telegram, WhatsApp) via the OnFire
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
	ID      string    `json:"id"`
	Owner   string    `json:"owner"`
	Title   string    `json:"title"`
	When    time.Time `json:"when"`
	Note    string    `json:"note,omitempty"`
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

	if err := service.Register("events", new(Server)); err != nil {
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
		Created: time.Now().UTC(),
	}
	mu.Lock()
	events[e.ID] = e
	saveLocked()
	mu.Unlock()
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
	}
}
