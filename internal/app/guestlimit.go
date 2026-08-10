package app

// What stops an open door being an abused one.
//
// Free tools are reachable without an account, which is the point: an agent
// finds the endpoint mid-task, tries something, and it works. That is also a
// standing invitation to anyone with a loop, and the credit charge is no
// defence because the charge is zero — that is what free means.
//
// So the two jobs stay separate, the way the cost block in internal/quota says they
// should: credits price what a call costs us, and a rate limit stops a bot.
// This is the second one, for callers we cannot name. A signed-in caller is
// already accountable — they have a balance, and CheckPostRate governs what
// they write — so the limit is only for guests.

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type guestBucket struct {
	count   int
	resetAt time.Time
}

var (
	guestMu    sync.Mutex
	guestCalls = map[string]*guestBucket{}
)

// GuestCallAllowed reports whether an unauthenticated caller at this IP may
// make another free tool call, and counts it when they may.
//
// Generous on purpose. This is the first thing a new agent does, often several
// times in a row while somebody is wiring it up, and a limit that trips during
// evaluation is indistinguishable from a broken endpoint. It exists to stop a
// loop, not to ration a trial.
//
// GUEST_MAX_PER_IP (default 120) per GUEST_WINDOW_MINUTES (default 60).
func GuestCallAllowed(ip string) bool {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return true // never rate-limit localhost (self-hosted, dev)
	}
	maxPerIP := EnvInt("GUEST_MAX_PER_IP", 120)
	window := time.Duration(EnvInt("GUEST_WINDOW_MINUTES", 60)) * time.Minute

	guestMu.Lock()
	defer guestMu.Unlock()

	now := time.Now()
	b, ok := guestCalls[ip]
	if !ok || now.After(b.resetAt) {
		b = &guestBucket{resetAt: now.Add(window)}
		guestCalls[ip] = b
	}
	if b.count >= maxPerIP {
		return false
	}
	b.count++

	// Opportunistic GC, the same as the signup bucket above it.
	if len(guestCalls) > 10000 {
		for k, v := range guestCalls {
			if now.After(v.resetAt) {
				delete(guestCalls, k)
			}
		}
	}
	return true
}

// resetGuestCalls and expireGuestBucket exist for the tests, which have to be
// able to start from nothing and to make a window elapse without waiting an
// hour. Kept beside the thing they manipulate rather than reaching into it
// from the test file, so the coupling is visible from here.
func resetGuestCalls() {
	guestMu.Lock()
	defer guestMu.Unlock()
	guestCalls = map[string]*guestBucket{}
}

func expireGuestBucket(ip string) {
	guestMu.Lock()
	defer guestMu.Unlock()
	if b, ok := guestCalls[ip]; ok {
		b.resetAt = time.Now().Add(-time.Minute)
	}
}

func EnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}
