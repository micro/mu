package hazards

import (
	"fmt"
	"sync"
	"time"
)

const refreshEvery = 5 * time.Minute

// Each public feed refreshes independently. Readers never wait for upstream.
type feedCache[T any] struct {
	mu            sync.Mutex
	items         []T
	at, attempted time.Time
	running       bool
	failed        bool
	fetch         func() ([]T, error)
}

func (c *feedCache[T]) read() ([]T, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running && time.Since(c.attempted) >= refreshEvery {
		c.running = true
		c.attempted = time.Now()
		go func() {
			items, err := c.fetch()
			c.mu.Lock()
			defer c.mu.Unlock()
			c.running = false
			c.failed = err != nil
			if err == nil {
				c.items = items
				c.at = time.Now().UTC()
			}
		}()
	}
	return append([]T(nil), c.items...), c.at, c.failed
}

var quakeCache = feedCache[quake]{fetch: func() ([]quake, error) { return recent(pageMinMagnitude, pagePeriod, 0, 0, 0) }}
var alertCache = feedCache[alert]{fetch: func() ([]alert, error) { return alerts("orange", 0, 0, 0) }}
var floodCache = feedCache[flood]{fetch: func() ([]flood, error) { return floodsNow(0, 0, 0, false) }}
var backgroundOnce sync.Once

func startRefresh() {
	backgroundOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(refreshEvery)
			defer ticker.Stop()
			for {
				gather()
				<-ticker.C
			}
		}()
	})
}

func freshness(at time.Time, failed bool) string {
	if at.IsZero() {
		if failed {
			return "Feed unavailable. Retrying in the background."
		}
		return "Loading this feed in the background. Refresh shortly."
	}
	note := fmt.Sprintf("Last fetched %s UTC.", at.UTC().Format("2 Jan 15:04"))
	if failed || time.Since(at) > 2*refreshEvery {
		note += " Refresh delayed; these results may be out of date."
	}
	return note
}

func feedInfo[T any](c *feedCache[T]) map[string]interface{} {
	_, at, failed := c.read()
	return map[string]interface{}{"fetched_at": at, "refresh_failed": failed, "status": freshness(at, failed)}
}
