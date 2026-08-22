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
	// Data: message, holding the whole InboundMail as JSON.
	EventMailReceived = "mail_received"

	// EventMailForAgent is a message that may wake an agent.
	//
	// A second topic rather than a flag on the first, and that is the whole
	// security design. service/mail decides — it is the only place that has the
	// SPF and DKIM results, and the rule is mayDispatch — and it publishes here
	// only when the answer is yes. A subscriber cannot see a message that
	// failed the gate, because it was never published.
	//
	// The alternative was one topic carrying "mayWake": true, which turns a
	// guarantee into a convention. Forgetting to check a field is a normal
	// mistake and it fails open, on the path where a stranger's mail drives
	// somebody's agent and spends their credits. Forgetting to subscribe to a
	// topic fails closed.
	//
	// Data: message, holding the whole InboundMail as JSON.
	EventMailForAgent = "mail_for_agent"

	// EventWorkForAgent is a piece of work somebody has asked an agent to do.
	//
	// Four things ask: a chat message, an email arriving, a task assigned, a
	// schedule firing. They were four mechanisms — and three of them were
	// hooks, a service reaching up into the agent through a function variable
	// somebody filled in at boot, because a service may not import an agent.
	// The hook is how that rule was avoided rather than kept.
	//
	// This is the same inversion service/mail already made, for the same
	// reason. A service does not run an agent; it says what happened, and
	// whoever wants to act on it subscribes. See agent/work, which is the
	// subscriber, and agent/mail, which is the worked example.
	//
	// Data: account, kind, id, title, prompt. kind and id name the record the
	// work belongs to, so the subscriber knows where the answer goes — a task
	// keeps its result, a standing instruction is mailed. That knowledge is the
	// agent layer's: an agent may import a service, and the service must not
	// know an agent exists.
	EventWorkForAgent = "work_for_agent"

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

// RequestWork asks an agent to do a piece of work, for whoever is listening.
//
// account is whose work it is, and the run is charged to them. kind and id name
// the record it belongs to — "task" and a task id, "event" and an event id —
// so the answer can be put back where it came from. title is what to call it;
// prompt is what to do.
//
// Nothing here knows what an agent is, which is the point: a service that
// called one would be asking the model what its own answer should be. See
// EventWorkForAgent.
func RequestWork(account, kind, id, title, prompt string) {
	if account == "" || prompt == "" {
		return
	}
	Publish(Event{Type: EventWorkForAgent, Data: map[string]interface{}{
		"account": account,
		"kind":    kind,
		"id":      id,
		"title":   title,
		"prompt":  prompt,
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
