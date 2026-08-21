// Package event provides a pub/sub event system for decoupling background
// operations across packages.
//
// It is a thin wrapper over the go-micro broker (mu/internal/service): there is
// one bus for the whole system, not a hand-rolled one beside the framework's.
// The channel-based API (Subscribe → Subscription.Chan, Publish, Close) is
// preserved so callers are unchanged. Event payloads are JSON-encoded onto the
// broker; all consumers read string values, so the round-trip is lossless.
package event

import (
	"encoding/json"
	"sync"

	"go-micro.dev/v6/broker"

	"mu/internal/service"
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

	// EventMailReceived is a non-spam message delivered to a local account.
	//
	// service/mail publishes it and knows nothing about who cares. It used to
	// be mail.OnNewMail, a function variable the service declared and
	// internal/server filled in — one of six hooks that existed so a service
	// could reach up into the agent, which is the direction the layering
	// forbids: a service answers a question about state, an agent decides
	// which question to ask.
	//
	// Mail arriving is a fact, not a call. Anything that wants to act on it
	// subscribes.
	//
	// Data: account, from, subject, body.
	EventMailReceived = "mail_received"

	// EventActivity is one thing that happened, in a line, with somewhere to
	// go and read it: a post published, a video found, a headline broken, an
	// image generated.
	//
	// It is what service/stream renders, and the reason it is a fact on the
	// bus rather than a call into stream is the rule about sideways imports —
	// a service that called stream would be two services read together,
	// changed together and moved together. Announcing costs the publisher
	// nothing and names nobody: stream is one subscriber and there is room
	// for others.
	//
	// Data: service, text, url, account. An empty account means everybody may
	// see it; a set one means only that account may.
	EventActivity = "activity"
)

// Announce records one thing that happened, for whoever is listening.
//
// service is the name it came from, which is what the timeline shows it under
// and where the icon comes from. url may be empty. account should be set only
// when the fact is somebody's own — a message that arrived, an image they
// generated — and then only that account ever sees it. Getting that wrong is
// how a public timeline came to be carrying people's mail; see purgePrivate in
// service/stream.
func Announce(service, text, url, account string) {
	if service == "" || text == "" {
		return
	}
	Publish(Event{Type: EventActivity, Data: map[string]interface{}{
		"service": service,
		"text":    text,
		"url":     url,
		"account": account,
	}})
}

// Event represents a data event.
type Event struct {
	Type string
	Data map[string]interface{}
}

// Subscription represents an active subscription. Callers range over Chan.
type Subscription struct {
	Chan chan Event

	sub    broker.Subscriber
	mu     sync.Mutex
	closed bool
}

// Subscribe creates a channel-based subscription for a specific event type,
// backed by a broker subscription on that topic.
func Subscribe(eventType string) *Subscription {
	s := &Subscription{Chan: make(chan Event, 10)}

	sub, err := service.Broker().Subscribe(eventType, func(e broker.Event) error {
		var data map[string]interface{}
		if m := e.Message(); m != nil && len(m.Body) > 0 {
			_ = json.Unmarshal(m.Body, &data)
		}
		ev := Event{Type: eventType, Data: data}

		// Non-blocking send, guarded so a concurrent Close can't cause a send on
		// a closed channel. Preserves the original drop-if-full semantics.
		s.mu.Lock()
		if !s.closed {
			select {
			case s.Chan <- ev:
			default:
			}
		}
		s.mu.Unlock()
		return nil
	})
	if err == nil {
		s.sub = sub
	}
	return s
}

// Close stops delivery and closes the channel so a ranging consumer exits.
func (s *Subscription) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.Chan)
	}
	s.mu.Unlock()

	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
}

// Publish sends an event to all subscribers of its type via the broker.
func Publish(e Event) {
	body, err := json.Marshal(e.Data)
	if err != nil {
		body = nil
	}
	_ = service.Broker().Publish(e.Type, &broker.Message{Body: body})
}
