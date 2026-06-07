package agent

import (
	"sync"
	"time"
)

const guestDailyLimit = 3

var (
	guestMu     sync.Mutex
	guestCounts = map[string]*guestBucket{}
)

type guestBucket struct {
	count   int
	resetAt time.Time
}

func guestQueryAllowed(ip string) bool {
	guestMu.Lock()
	defer guestMu.Unlock()

	b, ok := guestCounts[ip]
	if !ok || time.Now().After(b.resetAt) {
		return true
	}
	return b.count < guestDailyLimit
}

func guestQueryRecord(ip string) {
	guestMu.Lock()
	defer guestMu.Unlock()

	b, ok := guestCounts[ip]
	if !ok || time.Now().After(b.resetAt) {
		guestCounts[ip] = &guestBucket{
			count:   1,
			resetAt: time.Now().Add(24 * time.Hour),
		}
		return
	}
	b.count++
}
