package video

// Rationing YouTube search.
//
// video_search used to cost 2 credits. Nothing bills us for it — the YouTube
// Data API is free — so the charge was pricing scarcity rather than cost, and
// it rationed the wrong way round: the quota is a property of this instance,
// shared by everyone, but the price fell on whoever happened to search. A user
// with credits could exhaust the day's searches for everybody, and a user
// without credits was refused even when the day's quota was untouched.
//
// The scarcity is real. YouTube gives 10,000 units a day and charges 100 per
// search, so this instance gets about 100 searches a day across all users. Two
// limits express that directly: a per-account hourly cap so no single caller
// takes the day, and a global daily cap kept under the real ceiling so a run of
// searches degrades to "try later" instead of every video page breaking when
// the quota is spent on something else.

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"mu/internal/settings"
)

const (
	defaultSearchesPerAccountHour = 20
	// Under the ~100/day the API allows, leaving headroom for the channel
	// refresh loop, which spends units on the same quota.
	defaultSearchesPerDay = 80
)

type searchBucket struct {
	count   int
	resetAt time.Time
}

var (
	searchMu      sync.Mutex
	perAccount    = map[string]*searchBucket{}
	globalBucket  = &searchBucket{}
	searchLimited = func(limit int, window string) error {
		return fmt.Errorf("video search is limited to %d per %s on this instance — try again shortly", limit, window)
	}
)

// searchLimits reads both caps. The settings keys are written out as literals
// rather than passed through a helper, so the scanner in docs/config_test.go
// can see them and hold the configuration page to documenting them. That
// scanner reads source text, so naming a key anywhere in this file — including
// in a comment — is a claim that the code reads it.
func searchLimits() (perHour, perDay int) {
	return limitOr(settings.Get("VIDEO_SEARCH_PER_HOUR"), defaultSearchesPerAccountHour),
		limitOr(settings.Get("VIDEO_SEARCH_PER_DAY"), defaultSearchesPerDay)
}

func limitOr(v string, def int) int {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return def
}

// allowSearch reserves one YouTube search for this account, or explains why it
// cannot. Both buckets are reserved together so a rejected call spends nothing.
func allowSearch(accountID string) error {
	perHour, perDay := searchLimits()

	searchMu.Lock()
	defer searchMu.Unlock()
	now := time.Now()

	if now.After(globalBucket.resetAt) {
		globalBucket.count, globalBucket.resetAt = 0, now.Add(24*time.Hour)
	}
	if globalBucket.count >= perDay {
		return searchLimited(perDay, "day across this instance")
	}

	b, ok := perAccount[accountID]
	if !ok || now.After(b.resetAt) {
		b = &searchBucket{resetAt: now.Add(time.Hour)}
		perAccount[accountID] = b
	}
	if b.count >= perHour {
		return searchLimited(perHour, "hour")
	}

	b.count++
	globalBucket.count++
	return nil
}
