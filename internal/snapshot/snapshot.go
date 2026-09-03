// Package snapshot implements the go-micro read-plane pattern for display cards
// .
//
// A producer Publishes a rendered HTML snapshot, which is written to the shared
// go-micro store (durable) and announced on the broker. The read path reads a
// broker-fed in-memory mirror via Get — a memory read, no per-render RPC
// fan-out. In-process today the producer and consumer are the same binary; the
// same flow works across processes once the registry is networked.
//
// Each display service owns one Snapshot, created in Load:
//
//	var card *snapshot.Snapshot
//	func Load() { card = snapshot.New("markets"); ... }
//	// on every cache rebuild: card.Publish(html)
//	// in the serve path: if s := card.Get(); s != "" { return s }  // else fallback
package snapshot

import (
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/store"

	"mu/internal/app"
	"mu/internal/service"
)

// Snapshot is one channel over the go-micro store + broker: a named service
// publishing one kind of thing.
//
// Cards were the only kind for a while, and the second is a service saying what
// it already knows in words, for an agent's prompt — see service.Spec.Now. Same
// plane, different payload: one is a page, the other is a fact.
type Snapshot struct {
	name   string // log/service name, e.g. "markets"
	kind   string // what is published, e.g. "card" or "now"
	key    string // store key, e.g. "markets:card"
	topic  string // broker topic, e.g. "markets.card"
	mu     sync.RWMutex
	mirror string
}

// New creates a snapshot channel for a named card and starts consuming updates:
// it primes the mirror from the store (covers a restart) and subscribes to the
// broker. Call once, from the package's Load.
func New(name string) *Snapshot { return Channel(name, "card") }

// Channel is the read-plane channel for one kind of thing a service publishes,
// created once and reused: creating one subscribes to the broker, and a second
// subscription to the same topic is a second handler doing identical work.
//
// Reused rather than left to callers to hold, because both sides of the plane
// want one — the service that publishes and, for the "now" kind, the agent that
// reads. Handing each a channel of its own would double the subscriptions and
// leave two mirrors of the same value.
func Channel(name, kind string) *Snapshot {
	channelsMu.Lock()
	defer channelsMu.Unlock()
	id := name + ":" + kind
	if s, ok := channels[id]; ok {
		return s
	}
	s := newChannel(name, kind)
	channels[id] = s
	return s
}

// Value is what a service last published of this kind, or "" if nothing has.
func Value(name, kind string) string { return Channel(name, kind).Get() }

var (
	channelsMu sync.Mutex
	channels   = map[string]*Snapshot{}
)

func newChannel(name, kind string) *Snapshot {
	s := &Snapshot{name: name, kind: kind, key: name + ":" + kind, topic: name + "." + kind}
	if recs, err := service.Store().Read(s.key); err == nil && len(recs) > 0 {
		s.set(string(recs[0].Value))
	}
	if _, err := service.Broker().Subscribe(s.topic, s.handle); err != nil {
		app.Log(s.name, "snapshot subscribe failed: %v", err)
	}
	return s
}

func (s *Snapshot) handle(e broker.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			app.Log(s.name, "snapshot handler panic: %v", r)
		}
	}()
	if m := e.Message(); m != nil {
		s.set(string(m.Body))
	}
	return nil
}

// Publish records the snapshot to the store and announces it on the broker.
// Best-effort: failures are logged, never block the caller's refresh.
func (s *Snapshot) Publish(value string) {
	if s == nil || value == "" {
		return
	}
	// Written to the mirror as well as sent.
	//
	// The broker delivers to subscribers, and the publisher is one of them only
	// by accident of being in the same process — on a memory broker it is, and
	// the moment this is two processes the publisher's own reads would answer
	// from a mirror nothing had filled. Setting it here means a service can
	// always read back what it just published.
	s.set(value)
	if err := service.Store().Write(&store.Record{Key: s.key, Value: []byte(value)}); err != nil {
		app.Log(s.name, "snapshot store write failed: %v", err)
	}
	if err := service.Broker().Publish(s.topic, &broker.Message{Body: []byte(value)}); err != nil {
		app.Log(s.name, "snapshot publish failed: %v", err)
	}
}

func (s *Snapshot) set(value string) {
	if value == "" {
		return
	}
	s.mu.Lock()
	s.mirror = value
	s.mu.Unlock()
}

// Get returns the latest broker-fed snapshot, or "" if none has arrived (the
// caller should fall back to its locally-generated HTML).
func (s *Snapshot) Get() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mirror
}
