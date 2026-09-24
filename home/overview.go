package home

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/events"
	"mu/service/weather"
)

type overviewSnapshot struct {
	key   string
	cards map[string]string
	apps  []apps.SavedApp
	at    time.Time
}

var overviewCache = struct {
	sync.Mutex
	values  map[string]overviewSnapshot
	pending map[string]*auth.Account
	queue   chan *auth.Account
	once    sync.Once
}{values: map[string]overviewSnapshot{}, pending: map[string]*auth.Account{}, queue: make(chan *auth.Account, 64)}

// Load starts a bounded background refresh pool. It never waits for a service
// or restores private data on the HTTP startup path.
func Load() {
	overviewCache.once.Do(func() {
		for i := 0; i < 2; i++ {
			go refreshOverviews()
		}
	})
}

func overviewKey(acc *auth.Account) string {
	return fmt.Sprintf("%s|%s|%g|%g|%s|%s", acc.ID, acc.Place, acc.Lat, acc.Lon, acc.Zone, strings.Join(acc.PinnedServices(), ","))
}

func overview(acc *auth.Account) (overviewSnapshot, bool) {
	Load()
	key := overviewKey(acc)
	overviewCache.Lock()
	defer overviewCache.Unlock()
	cached := overviewCache.values[acc.ID]
	if cached.key != key {
		cached = overviewSnapshot{}
	}
	if time.Since(cached.at) < 2*time.Minute {
		return cached, false
	}
	if overviewCache.pending[acc.ID] == nil {
		copy := *acc
		copy.Pinned = append([]string{}, acc.PinnedServices()...)
		select {
		case overviewCache.queue <- &copy:
			overviewCache.pending[acc.ID] = &copy
		default:
		}
	}
	return cached, true
}

func refreshOverviews() {
	for acc := range overviewCache.queue {
		snapshot := overviewSnapshot{key: overviewKey(acc), cards: map[string]string{}}
		var collection apps.CollectionResponse
		ctx, cancel := context.WithTimeout(service.WithAccount(context.Background(), acc.ID), 10*time.Second)
		if service.Call(ctx, "apps", "Server.Collection", &apps.CollectionRequest{}, &collection) == nil {
			snapshot.apps = collection.Items
		}
		cancel()
		for _, spec := range service.Pinned(acc.PinnedServices()) {
			// The unlocated weather renderer requires an inline browser script.
			// Home already offers the account location control instead.
			if spec.Name == "weather" && acc.Lat == 0 && acc.Lon == 0 {
				continue
			}
			if spec.Card.Set() {
				snapshot.cards[spec.Name] = spec.Card.Render(service.For(acc.ID)).HTML
			}
		}
		if acc.Lat != 0 || acc.Lon != 0 {
			weather.Warm(acc.Lat, acc.Lon)
		}
		if !events.OverviewFresh(acc.ID) {
			events.Overview(acc.ID, events.PreviewLimit)
		}
		snapshot.at = time.Now()
		overviewCache.Lock()
		if overviewCache.pending[acc.ID] != acc {
			overviewCache.Unlock()
			continue
		}
		// Bound idle account caches without flushing other accounts' useful views.
		if len(overviewCache.values) >= 256 {
			oldest := ""
			var at time.Time
			for owner, value := range overviewCache.values {
				if oldest == "" || value.at.Before(at) {
					oldest, at = owner, value.at
				}
			}
			delete(overviewCache.values, oldest)
		}
		overviewCache.values[acc.ID] = snapshot
		delete(overviewCache.pending, acc.ID)
		overviewCache.Unlock()
	}
}

// Forget invalidates in-flight work too, so a deleted account cannot regain a
// private snapshot when a background renderer finishes.
func Forget(owner string) {
	overviewCache.Lock()
	delete(overviewCache.values, owner)
	delete(overviewCache.pending, owner)
	overviewCache.Unlock()
}
