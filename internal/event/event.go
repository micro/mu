// Package event provides a pub/sub event system for decoupling
// background operations across packages.
package event

import (
	"fmt"
	"sync"
)

// Event types
const (
	EventRefreshHNComments  = "refresh_hn_comments"
	EventIndexComplete      = "index_complete"
	EventNewArticleMetadata = "new_article_metadata"
	EventGenerateSummary    = "generate_summary"
	EventSummaryGenerated   = "summary_generated"
	EventGenerateTag        = "generate_tag"
	EventTagGenerated       = "tag_generated"
	EventTaskCreated        = "task_created"
	EventTaskRetry          = "task_retry"
)

// Event represents a data event
type Event struct {
	Type string
	Data map[string]interface{}
}

// Subscription represents an active subscription
type Subscription struct {
	Chan      chan Event
	eventType string
	id        string
}

var (
	mu              sync.RWMutex
	subscribers     = make(map[string]map[string]chan Event) // eventType -> subscriberID -> channel
	subscriberIDSeq int
)

// Subscribe creates a channel-based subscription for a specific event type
func Subscribe(eventType string) *Subscription {
	mu.Lock()
	defer mu.Unlock()

	// Generate unique subscriber ID
	subscriberIDSeq++
	id := fmt.Sprintf("sub_%d", subscriberIDSeq)

	// Create buffered channel to prevent blocking
	ch := make(chan Event, 10)

	// Initialize map if needed
	if subscribers[eventType] == nil {
		subscribers[eventType] = make(map[string]chan Event)
	}

	subscribers[eventType][id] = ch

	return &Subscription{
		Chan:      ch,
		eventType: eventType,
		id:        id,
	}
}

// Close closes the channel and removes the subscription
func (s *Subscription) Close() {
	mu.Lock()
	defer mu.Unlock()

	if subs, ok := subscribers[s.eventType]; ok {
		if ch, ok := subs[s.id]; ok {
			close(ch)
			delete(subs, s.id)
		}
	}
}

// Publish sends an event to all subscribers
func Publish(e Event) {
	mu.RLock()
	subs := subscribers[e.Type]
	mu.RUnlock()

	// Send to channel subscribers (non-blocking)
	for _, ch := range subs {
		select {
		case ch <- e:
			// Sent successfully
		default:
			// Channel full, skip (subscriber should have buffer or be reading)
		}
	}
}
