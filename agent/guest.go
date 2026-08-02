package agent

import (
	"sync"
	"time"

	"mu/internal/service"
)

const guestDailyLimit = 3

// guestExtraTools are the tools a guest may use that have no service behind
// them, so service.GuestAllowedTool cannot answer for them. Everything that is
// service-backed is derived from whether that service is account-scoped.
var guestExtraTools = map[string]bool{
	"quran":         true,
	"quran_search":  true,
	"hadith":        true,
	"blog_read":     true,
	"social_search": true,
	"video_search":  true,
	"apps_run":      true,
}

func isGuestAllowedTool(name string) bool {
	return service.GuestAllowedTool(name) || guestExtraTools[name]
}

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
