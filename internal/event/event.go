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

	// MailForAgent is a message that may wake an agent.
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
	MailForAgent = "mail_for_agent"

	// WorkForAgent is a piece of work somebody has asked an agent to do.
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
	// Data: account, kind, id, title, prompt, thread. kind and id name the record the
	// work belongs to, so the subscriber knows where the answer goes — a task
	// keeps its result, a standing instruction is mailed. That knowledge is the
	// agent layer's: an agent may import a service, and the service must not
	// know an agent exists.
	WorkForAgent = "work_for_agent"

	// ChatForAgent is something said in a room where an agent is expected to
	// answer.
	//
	// A second topic rather than a flag, for the reason MailForAgent gives:
	// service/chat holds the facts that decide — who is in the room, whether
	// the agent was named, what kind of room it is — and publishes here only
	// when the answer is yes. A subscriber cannot see a message that failed
	// the gate.
	//
	// It exists because the chat service was composing the replies itself:
	// its own RAG over the index, its own decision about whether to search the
	// web, its own history assembled from the room, and a model call, in a
	// websocket goroutine. That is a hand-rolled agent inside a service, and
	// the rule it broke is the one with a reason rather than a convention
	// behind it — a service answers a question about state, an agent decides
	// which question to ask.
	//
	// The replacement is not a smaller version of the same thing. An agent
	// woken here reaches every tool this instance has rather than the two that
	// were wired in by hand.
	//
	// Data: room, title, summary, url, account, text. The room fields are what
	// the conversation is about, which the subscriber cannot look up without
	// importing the service back.
	ChatForAgent = "chat_for_agent"

	// SMSForAgent is a text from a number that has proved whose it is, which an
	// agent should answer. The same shape as ChatForAgent and for the same
	// reason: service/sms owns the number and may not call an agent, so it says
	// what arrived and agent/sms answers.
	//
	// Published only for a sender the account knows — a verified number, or one
	// this instance texted first. Never for the fallback owner, because that is
	// the operator and every stranger who dials the number would otherwise be
	// talking to their agent on their credits.
	//
	// Data: owner, from, text.
	SMSForAgent = "sms_for_agent"

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
	// the rule agent/work exists to honour and the one this replaces a
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

// RequestWork asks an agent to do a piece of work, for whoever is listening.
//
// account is whose work it is, and the run is charged to them. kind and id name
// the record it belongs to — "task" and a task id, "event" and an event id —
// so the answer can be put back where it came from. title is what to call it;
// prompt is what to do.
//
// thread is the conversation the work came out of, where there was one. Work
// does not usually start on a page: somebody writes in and asks for something
// that takes an hour, and the answer belongs where they asked rather than on a
// record they have no reason to check. Empty for work nobody asked for in
// conversation — a task written down on the page, a schedule falling due.
//
// Nothing here knows what an agent is, which is the point: a service that
// called one would be asking the model what its own answer should be. See
// WorkForAgent.
// RequestChatReply says something was said in a room and an agent is expected
// to answer it.
//
// Called only once the room has decided — see ChatForAgent. Nothing here knows
// what an agent is, and nothing in the chat service needs to.
func RequestChatReply(room, title, summary, url, account, text string) {
	if room == "" || text == "" {
		return
	}
	Publish(Event{Type: ChatForAgent, Data: map[string]interface{}{
		"room": room,
		// What the room is about, carried rather than looked up: a subscriber
		// that fetched it would be importing the service it is decoupled from.
		"title":   title,
		"summary": summary,
		"url":     url,
		"account": account,
		"text":    text,
	}})
}

func RequestWork(account, kind, id, title, prompt, thread, agent string) {
	if account == "" || prompt == "" {
		return
	}
	Publish(Event{Type: WorkForAgent, Data: map[string]interface{}{
		"account": account,
		"kind":    kind,
		"id":      id,
		"title":   title,
		"prompt":  prompt,
		"thread":  thread,
		// Which agent, when the work was given to a particular one. An opaque
		// name here: nothing in this package knows what an agent is, and the
		// subscriber is what resolves it. Empty means whichever one the
		// instance runs by default, which is every caller but the inbox today.
		"agent": agent,
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
