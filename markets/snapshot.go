package markets

import (
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/store"

	"mu/internal/app"
	"mu/internal/service"
)

// This is the reference implementation of the go-micro read-plane pattern
// described in internal/docs/ARCHITECTURE.md.
//
// The service PRODUCES a snapshot (the rendered card), writes it to the shared
// go-micro store (durable, survives restart) and announces it on the broker. The
// read path CONSUMES a broker-fed in-memory mirror, so a render is a memory read
// — no per-render RPC fan-out — and the data flows entirely over go-micro
// primitives. In-process today the producer and consumer are the same binary;
// the same flow works across processes once the registry is networked.
//
// Other display services should follow this shape (snapshot.go per service).

const (
	snapshotStoreKey = "markets:card"
	snapshotTopic    = "markets.card"
)

var (
	snapshotMu     sync.RWMutex
	snapshotMirror string
)

// publishSnapshot records the latest rendered card to the store and announces it
// on the broker. Best-effort: failures are logged, never block a refresh.
func publishSnapshot(html string) {
	if html == "" {
		return
	}
	if err := service.Store().Write(&store.Record{Key: snapshotStoreKey, Value: []byte(html)}); err != nil {
		app.Log("markets", "snapshot store write failed: %v", err)
	}
	if err := service.Broker().Publish(snapshotTopic, &broker.Message{Body: []byte(html)}); err != nil {
		app.Log("markets", "snapshot publish failed: %v", err)
	}
}

// startSnapshotConsumer primes the mirror from the durable store and subscribes
// to snapshot updates on the broker. Called once from Load.
func startSnapshotConsumer() {
	// Prime from the store (covers a restart / late join before the first refresh).
	if recs, err := service.Store().Read(snapshotStoreKey); err == nil && len(recs) > 0 {
		setSnapshotMirror(string(recs[0].Value))
	}
	if _, err := service.Broker().Subscribe(snapshotTopic, snapshotHandler); err != nil {
		app.Log("markets", "snapshot subscribe failed: %v", err)
	}
}

// snapshotHandler updates the mirror from a broker event. Guarded so a bad
// payload can never crash the publisher's goroutine.
func snapshotHandler(e broker.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			app.Log("markets", "snapshot handler panic: %v", r)
		}
	}()
	if m := e.Message(); m != nil {
		setSnapshotMirror(string(m.Body))
	}
	return nil
}

func setSnapshotMirror(html string) {
	if html == "" {
		return
	}
	snapshotMu.Lock()
	snapshotMirror = html
	snapshotMu.Unlock()
}

// snapshot returns the latest broker-fed snapshot, or "" if none has arrived.
func snapshot() string {
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	return snapshotMirror
}
