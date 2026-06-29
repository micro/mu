// Package snapshot implements the go-micro read-plane pattern for display cards
// (see internal/docs/ARCHITECTURE.md).
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

// Snapshot is one card's read-plane channel over the go-micro store + broker.
type Snapshot struct {
	name   string // log/service name, e.g. "markets"
	key    string // store key, e.g. "markets:card"
	topic  string // broker topic, e.g. "markets.card"
	mu     sync.RWMutex
	mirror string
}

// New creates a snapshot channel for a named card and starts consuming updates:
// it primes the mirror from the store (covers a restart) and subscribes to the
// broker. Call once, from the package's Load.
func New(name string) *Snapshot {
	s := &Snapshot{name: name, key: name + ":card", topic: name + ".card"}
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
func (s *Snapshot) Publish(html string) {
	if s == nil || html == "" {
		return
	}
	if err := service.Store().Write(&store.Record{Key: s.key, Value: []byte(html)}); err != nil {
		app.Log(s.name, "snapshot store write failed: %v", err)
	}
	if err := service.Broker().Publish(s.topic, &broker.Message{Body: []byte(html)}); err != nil {
		app.Log(s.name, "snapshot publish failed: %v", err)
	}
}

func (s *Snapshot) set(html string) {
	if html == "" {
		return
	}
	s.mu.Lock()
	s.mirror = html
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
