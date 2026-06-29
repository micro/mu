package news

import (
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/store"

	"mu/internal/app"
	"mu/internal/service"
)

// Read-plane snapshot for the news card — same pattern as markets/snapshot.go
// (see internal/docs/ARCHITECTURE.md): produce a snapshot, write it to the
// go-micro store and announce it on the broker; the read path (Headlines) serves
// a broker-fed in-memory mirror, so a render is a memory read with no per-render
// RPC fan-out.

const (
	snapshotStoreKey = "news:card"
	snapshotTopic    = "news.card"
)

var (
	snapshotMu     sync.RWMutex
	snapshotMirror string
)

// publishSnapshot records the latest rendered headlines to the store and
// announces them on the broker. Best-effort; failures are logged.
func publishSnapshot(html string) {
	if html == "" {
		return
	}
	if err := service.Store().Write(&store.Record{Key: snapshotStoreKey, Value: []byte(html)}); err != nil {
		app.Log("news", "snapshot store write failed: %v", err)
	}
	if err := service.Broker().Publish(snapshotTopic, &broker.Message{Body: []byte(html)}); err != nil {
		app.Log("news", "snapshot publish failed: %v", err)
	}
}

// startSnapshotConsumer primes the mirror from the store and subscribes to
// snapshot updates on the broker. Called once from Load.
func startSnapshotConsumer() {
	if recs, err := service.Store().Read(snapshotStoreKey); err == nil && len(recs) > 0 {
		setSnapshotMirror(string(recs[0].Value))
	}
	if _, err := service.Broker().Subscribe(snapshotTopic, snapshotHandler); err != nil {
		app.Log("news", "snapshot subscribe failed: %v", err)
	}
}

func snapshotHandler(e broker.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			app.Log("news", "snapshot handler panic: %v", r)
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
