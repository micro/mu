// Package event carries service facts. Durable records use Commit and Consume;
// older integrations and transient hints use Publish and Subscribe.
package event

import (
	"encoding/json"
	"strings"
	"sync"

	"go-micro.dev/v6/broker"

	"mu/internal/service"
)

// Event types
const (
	RefreshHNComments  = "refresh_hn_comments"
	IndexComplete      = "index_complete"
	NewArticleMetadata = "new_article_metadata"
	GenerateSummary    = "generate_summary"
	SummaryGenerated   = "summary_generated"
	GenerateTag        = "generate_tag"
	TagGenerated       = "tag_generated"

	// MailReceived is a non-spam message delivered to a local account.
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
	MailReceived = "mail_received"

	// SMSReceived is a text arriving, whoever it is from.
	//
	// The fact, with no gate on it, the way MailReceived is the fact and
	// MailAccepted is the permission. SMS had only the second, so a text from a
	// number nobody here knew was dropped with a log line: the only way it could
	// have been recorded was the side effect of an agent answering it, and the
	// agent is exactly what a stranger must not be able to start.
	//
	// Two topics rather than one with a flag, for the reason spelled out on
	// MailAccepted — a subscriber cannot forget to check a topic it is not
	// subscribed to.
	//
	// Known says whether the sender proved who they are. It is a fact about the
	// sender rather than a permission: what a subscriber does with an arrival
	// from a stranger is its own business, and holding it is one answer.
	//
	// Data: owner, from, text, known.
	SMSReceived = "sms_received"

	// ArrivalHeld is a conversation put in the record and not let in.
	//
	// Published by whatever recorded it, once, after the hold. A gatekeeper
	// subscribes and decides — see agent/gate — and this is the topic rather
	// than the channel's own because the question is identical whatever carried
	// it: a text from an unknown number, a federated chat from an unknown JID
	// and mail from a stranger are one question asked three times.
	//
	// It carries the thread rather than the message. The recorder has already
	// opened the conversation and put the words in it, so a subscriber that was
	// handed the text would have to find the thread again to act on it.
	//
	// Data: account, thread, client, from.
	ArrivalHeld = "arrival_held"

	// Activity is one thing that happened, in a line, with somewhere to
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
	Activity = "activity"

	// AccountCreated is a new account on this instance.
	//
	// A fact, deliberately, and not a notification. internal/auth knows an
	// account was made and has no business knowing that somebody wants to be
	// told about it — that is a product requirement, and a building block that
	// encodes one stops being reusable by anything that decides differently.
	// Whether it is worth waking an operator for is admin's judgement; see
	// admin/alert.go, which subscribes.
	//
	// The same inversion mail.OnNewMail made, for the same reason: the
	// alternative is a function variable somebody fills in at boot, which is an
	// import the compiler cannot see.
	//
	// Data: account, name, first ("true" when it is the instance's first).
	AccountCreated = "account_created"

	// ContentPublished is something somebody wrote that is now visible: a
	// post, a reply, an app description, an answer the agent gave in public.
	//
	// A fact, and deliberately not a request to moderate it. Whether it should
	// be hidden is a judgement made by a model, and a service asking for that
	// judgement is a service asking the model what its own answer should be —
	// the rule work exists to honour and the one this replaces a
	// violation of. service/social, service/blog and service/apps each called
	// a classifier through a function variable filled in at boot by, of all
	// things, service/chat: an import the compiler could not see, from three
	// services to a fourth, and moderation for the whole instance silently off
	// if that fourth one ever failed to load.
	//
	// Now they say what happened and stop. agent/moderate subscribes.
	//
	// Data: kind (the content type — "post", "social", "app"), id, title, text.
	ContentPublished = "content_published"
)

// Published says something somebody wrote is now visible, for whoever is
// listening.
//
// kind and id name it the way internal/flag names it, because whatever acts on
// this has to be able to point back at the thing — "social" and a thread id,
// "post" and a post id. title may be empty; text is what was written.
//
// Nothing here knows what moderation is, which is the point.
func Published(kind, id, title, text string) {
	if kind == "" || id == "" || strings.TrimSpace(text) == "" {
		return
	}
	Publish(Event{Type: ContentPublished, Data: map[string]interface{}{
		"kind":  kind,
		"id":    id,
		"title": title,
		"text":  text,
	}})
}

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
	Publish(Event{Type: Activity, Data: map[string]interface{}{
		"service": service,
		"text":    text,
		"url":     url,
		"account": account,
	}})
}

// AddressedChat reports a message explicitly addressed to the assistant.
func AddressedChat(room, title, summary, url, account, text string, refs ...string) {
	if room == "" || text == "" {
		return
	}
	ref := ""
	if len(refs) > 0 {
		ref = refs[0]
	}
	Publish(Event{Type: ChatAddressed, Data: map[string]interface{}{
		"room": room,
		"ref":  ref,
		// What the room is about, carried rather than looked up: a subscriber
		// that fetched it would be importing the service it is decoupled from.
		"title":   title,
		"summary": summary,
		"url":     url,
		"account": account,
		"text":    text,
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
