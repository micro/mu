package events

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/google"
)

type overviewSnapshot struct {
	entries []External
	expires time.Time
}

var overviewCache = struct {
	sync.Mutex
	values  map[string]overviewSnapshot
	pending map[string]chan struct{}
}{values: make(map[string]overviewSnapshot), pending: make(map[string]chan struct{})}

func overviewKey(owner string, limit int) string {
	account := ""
	if ExternalAccount != nil {
		account = ExternalAccount(owner)
	}
	return account + "\x00" + owner + "\x00" + strconv.Itoa(limit) + "\x00" + strings.Join(google.SelectedCalendars(owner), "\x00")
}

// CachedOverview provides the last calendar snapshot without waiting on a provider.
// Local events are always read directly by Preview.
func CachedOverview(owner string) []External {
	if owner == "" || (ExternalConnected != nil && !HasExternal(owner)) {
		return nil
	}
	key := overviewKey(owner, PreviewLimit)
	overviewCache.Lock()
	defer overviewCache.Unlock()
	return withoutLocalCopies(Upcoming(owner), futureEntries(overviewCache.values[key].entries))
}

func futureEntries(entries []External) []External {
	now := time.Now()
	var out []External
	for _, e := range entries {
		if e.Start.After(now) || (e.AllDay && e.End.After(now)) {
			out = append(out, e)
		}
	}
	return out
}

// Overview reuses a recent snapshot across Home and Events. The caller's account
// and selected calendars identify the snapshot; response HTML is never shared.
func Overview(owner string, limit int) []External {
	if owner == "" || (ExternalConnected != nil && !HasExternal(owner)) {
		return nil
	}
	key := overviewKey(owner, limit)
	overviewCache.Lock()
	snapshot, ok := overviewCache.values[key]
	if ok && time.Now().Before(snapshot.expires) {
		overviewCache.Unlock()
		return withoutLocalCopies(Upcoming(owner), futureEntries(snapshot.entries))
	}
	if ready := overviewCache.pending[key]; ready != nil {
		overviewCache.Unlock()
		<-ready
		return Overview(owner, limit)
	}
	ready := make(chan struct{})
	overviewCache.pending[key] = ready
	overviewCache.Unlock()
	defer func() { overviewCache.Lock(); delete(overviewCache.pending, key); close(ready); overviewCache.Unlock() }()
	now := time.Now()
	// Cache provider entries, not the merge: cancelling or editing a local
	// reminder must reveal its independent calendar copy immediately.
	entries := externalEvents(owner, now, now.Add(30*24*time.Hour), limit)
	overviewCache.Lock()
	if len(overviewCache.values) >= 512 {
		overviewCache.values = make(map[string]overviewSnapshot)
	}
	overviewCache.values[key] = overviewSnapshot{entries: append([]External(nil), entries...), expires: time.Now().Add(2 * time.Minute)}
	overviewCache.Unlock()
	return withoutLocalCopies(Upcoming(owner), entries)
}

// OverviewFresh reports whether Home can use its snapshot without a provider refresh.
func OverviewFresh(owner string) bool {
	if owner == "" || (ExternalConnected != nil && !HasExternal(owner)) {
		return true
	}
	key := overviewKey(owner, PreviewLimit)
	overviewCache.Lock()
	defer overviewCache.Unlock()
	snapshot, ok := overviewCache.values[key]
	return ok && time.Now().Before(snapshot.expires)
}
