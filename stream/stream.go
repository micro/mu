// Package stream is the platform-level event stream. Every building
// block publishes to it. Users interact with it via the console.
// The agent responds in it. It is the operational surface of Mu.
//
// This is NOT status updates (profile feature) and NOT social
// (threaded forum). It's a single append-only timeline of typed
// events that powers the home console.
package stream

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// Event types.
const (
	TypeUser     = "user"     // human typed in the console
	TypeAgent    = "agent"    // @micro response
	TypeSystem   = "system"   // mail notification, account event
	TypeMarket   = "market"   // price movement
	TypeNews     = "news"     // breaking headline
	TypeReminder = "reminder" // daily reminder
)

// Event is a single entry in the stream.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	AuthorID  string         `json:"author_id"`
	Author    string         `json:"author"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// MaxEvents is the number of events kept in memory / on disk.
const MaxEvents = 500

// MaxContentLength caps event content.
const MaxContentLength = 1024

var (
	mu     sync.RWMutex
	events []*Event // newest first

	// lastSystemEvent tracks the last time each system event type was
	// published. Used to throttle — the console is conversational, not
	// a news ticker. No lock needed, only accessed from Publish which
	// holds mu.
	lastSystemEvent = map[string]time.Time{}
)

// systemCooldown is the minimum interval between system events of the
// same type. Prevents flooding the stream with e.g. 10 headlines at
// once. User and agent events are never throttled.
var systemCooldown = 30 * time.Minute

func init() {
	b, err := data.LoadFile("stream.json")
	if err != nil {
		return
	}
	var loaded []*Event
	if json.Unmarshal(b, &loaded) == nil {
		mu.Lock()
		events = loaded
		mu.Unlock()
	}
}

// Load initialises the stream package.
func Load() {
	app.Log("stream", "Loaded %d events", len(events))
}

func save() {
	data.SaveJSON("stream.json", events)
}

// Publish appends an event to the stream. This is the single entry
// point — every publisher (user, agent, system, markets, news,
// reminder) calls this. System events are throttled per type so the
// console doesn't flood with automated content.
func Publish(e *Event) {
	if e.Content == "" {
		return
	}
	// Throttle system event types — max one per cooldown period.
	// User and agent events are never throttled.
	if e.Type != TypeUser && e.Type != TypeAgent {
		mu.RLock()
		last, exists := lastSystemEvent[e.Type]
		mu.RUnlock()
		if exists && time.Since(last) < systemCooldown {
			return // too soon, skip silently
		}
	}
	if len(e.Content) > MaxContentLength {
		e.Content = e.Content[:MaxContentLength-1] + "…"
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.Author == "" && e.AuthorID != "" {
		if acc, err := auth.GetAccount(e.AuthorID); err == nil {
			e.Author = acc.Name
		} else if e.AuthorID == app.SystemUserID {
			e.Author = app.SystemUserName
		} else {
			e.Author = e.AuthorID
		}
	}

	mu.Lock()
	if e.Type != TypeUser && e.Type != TypeAgent {
		lastSystemEvent[e.Type] = time.Now()
	}
	events = append([]*Event{e}, events...)
	if len(events) > MaxEvents {
		events = events[:MaxEvents]
	}
	save()
	mu.Unlock()
}

// PostUser is a convenience for human messages from the console.
func PostUser(accountID, content string) *Event {
	name := accountID
	if acc, err := auth.GetAccount(accountID); err == nil {
		name = acc.Name
	}
	e := &Event{
		Type:     TypeUser,
		AuthorID: accountID,
		Author:   name,
		Content:  content,
	}
	Publish(e)
	return e
}

// PostAgent is a convenience for @micro responses.
func PostAgent(content string) *Event {
	e := &Event{
		Type:     TypeAgent,
		AuthorID: app.SystemUserID,
		Author:   app.SystemUserName,
		Content:  content,
	}
	Publish(e)
	return e
}

// PostSystem posts a system notification (mail, account events).
func PostSystem(content string, meta map[string]any) *Event {
	e := &Event{
		Type:     TypeSystem,
		AuthorID: app.SystemUserID,
		Author:   app.SystemUserName,
		Content:  content,
		Metadata: meta,
	}
	Publish(e)
	return e
}

// Recent returns the most recent events, newest first, up to max.
// viewerID is used to include banned users' own posts.
func Recent(max int, viewerID string) []*Event {
	mu.RLock()
	defer mu.RUnlock()

	var result []*Event
	for _, e := range events {
		// Banned users' events are hidden from everyone except themselves.
		if e.Type == TypeUser && e.AuthorID != viewerID && auth.IsBanned(e.AuthorID) {
			continue
		}
		result = append(result, e)
		if len(result) >= max {
			break
		}
	}
	return result
}

// Since returns events newer than the given time.
func Since(since time.Time) []*Event {
	mu.RLock()
	defer mu.RUnlock()

	var result []*Event
	for _, e := range events {
		if !e.CreatedAt.After(since) {
			break // events are newest-first, so once we pass since we're done
		}
		result = append(result, e)
	}
	return result
}

// CountSince returns the number of events newer than since.
func CountSince(since time.Time) int {
	mu.RLock()
	defer mu.RUnlock()

	count := 0
	for _, e := range events {
		if !e.CreatedAt.After(since) {
			break
		}
		count++
	}
	return count
}

// DedupeAdjacent removes consecutive identical user+content pairs
// from a slice (display-time, doesn't modify storage).
func DedupeAdjacent(events []*Event) []*Event {
	if len(events) <= 1 {
		return events
	}
	result := []*Event{events[0]}
	for i := 1; i < len(events); i++ {
		prev := result[len(result)-1]
		cur := events[i]
		if cur.AuthorID == prev.AuthorID && cur.Content == prev.Content {
			continue
		}
		result = append(result, cur)
	}
	return result
}

// Clear wipes all events. Admin use only.
func Clear() {
	mu.Lock()
	events = nil
	save()
	mu.Unlock()
}

// ClearByAuthor removes all events from a specific author.
func ClearByAuthor(authorID string) {
	mu.Lock()
	var filtered []*Event
	for _, e := range events {
		if e.AuthorID != authorID {
			filtered = append(filtered, e)
		}
	}
	events = filtered
	save()
	mu.Unlock()
}

// All returns a sorted copy of all events (for admin/export).
func All() []*Event {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]*Event, len(events))
	copy(result, events)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// MicroMention is the trigger token for AI responses in the stream.
const MicroMention = "@micro"

// ContainsMicro checks for @micro mention with word boundaries.
func ContainsMicro(text string) bool {
	lower := strings.ToLower(text)
	idx := 0
	for {
		i := strings.Index(lower[idx:], MicroMention)
		if i < 0 {
			return false
		}
		pos := idx + i
		if pos > 0 {
			c := lower[pos-1]
			if isWordChar(c) {
				idx = pos + len(MicroMention)
				continue
			}
		}
		after := pos + len(MicroMention)
		if after < len(lower) && isWordChar(lower[after]) {
			idx = after
			continue
		}
		return true
	}
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}
