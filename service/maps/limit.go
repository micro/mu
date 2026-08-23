package maps

// How many tiles one account may make this instance go and fetch.
//
// Tiles are free, and that is a decision about price rather than about
// appetite. Nothing is charged, so nothing throttles — and the thing on the
// other side of a cold fetch is Ordnance Survey's bill, not ours to hand out
// without a limit. CLAUDE.md is explicit about which tool does which job:
// "Abuse control is auth.CheckPostRate, not the credit charge. Keep the two
// jobs separate: credits price real cost, rate limits stop bots."
//
// Not CheckPostRate itself, though. That is sixty an hour and it is the budget
// for writing — posts, replies, apps — so spending it on a basemap would mean
// looking at a map for a minute and then being unable to write anything. One
// pan is forty tiles. It is the right mechanism and the wrong bucket.
//
// So: a bucket of its own, counting the only thing that costs anything. A tile
// this instance already holds is served without touching this at all, however
// many times and to whoever asks, because serving it again is free in the sense
// that matters — it is free to us.

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/settings"
)

// coldPerHour is how many tiles one account may cause to be fetched in an hour.
//
// Two thousand is a region: a town at street zoom, or a national park at
// walking zoom, with room to move around it. It is not a mirror of Britain,
// which is what the limit is for — at this rate copying the country takes years
// rather than an afternoon, and an operator who wants to seed a region on
// purpose raises it for a day.
const coldPerHour = 2000

type bucket struct {
	n     int
	since time.Time
}

var (
	coldMu      sync.Mutex
	coldBuckets = map[string]*bucket{}
)

// coldLimit is the configured ceiling, so an operator can raise or lower it
// without a rebuild — seeding a region on purpose is a legitimate thing to want
// for an afternoon.
func coldLimit() int {
	if raw := strings.TrimSpace(settings.Get("TILE_FETCH_PER_HOUR")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return coldPerHour
}

// mayFetch reports whether this account may cause another cold fetch, and
// counts it when it may.
//
// A sliding bucket that resets an hour after the first fetch in it, which is
// the same shape auth.CheckPostRate uses — worth matching, because two limiters
// in one product that behave differently is two things to explain.
//
// An empty account is refused rather than pooled: an anonymous caller cannot
// reach a cold tile at all (see TileHandler), and a shared anonymous bucket
// would be one person able to exhaust it for everybody.
func mayFetch(accountID string) error {
	if accountID == "" {
		return fmt.Errorf("sign in to fetch a tile this instance has not seen before")
	}
	max := coldLimit()

	coldMu.Lock()
	defer coldMu.Unlock()

	now := time.Now()
	b, ok := coldBuckets[accountID]
	if !ok || now.Sub(b.since) >= time.Hour {
		coldBuckets[accountID] = &bucket{n: 1, since: now}
		return nil
	}
	if b.n >= max {
		left := time.Hour - now.Sub(b.since)
		return fmt.Errorf("that is %d new tiles this hour, which is the limit — tiles "+
			"already held are still free and unlimited, and this resets in %d minutes",
			max, int(left.Minutes())+1)
	}
	b.n++
	return nil
}

// forgetFetches drops the counters. Nothing calls it in the server; it is here
// so a test can start from a known state without reaching into the map.
func forgetFetches() {
	coldMu.Lock()
	defer coldMu.Unlock()
	coldBuckets = map[string]*bucket{}
}
