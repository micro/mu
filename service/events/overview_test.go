package events

import (
	"testing"
	"time"
)

func TestOverviewCachesByOwner(t *testing.T) {
	oldEntries, oldConnected := ExternalEntries, ExternalConnected
	defer func() { ExternalEntries = oldEntries; ExternalConnected = oldConnected }()
	ExternalConnected = func(string) bool { return true }
	calls := 0
	ExternalEntries = func(owner string, _, _ time.Time, limit int) []External {
		calls++
		if limit != PreviewLimit {
			t.Fatal("unbounded preview")
		}
		return []External{{Title: owner, Start: time.Now().Add(time.Hour)}}
	}
	a, b := "overview-test-a", "overview-test-b"
	if OverviewFresh(a) {
		t.Fatal("missing snapshot marked fresh")
	}
	Overview(a, PreviewLimit)
	Overview(a, PreviewLimit)
	if !OverviewFresh(a) || OverviewFresh(b) {
		t.Fatal("freshness crossed accounts")
	}
	Overview(b, PreviewLimit)
	if calls != 2 {
		t.Fatalf("provider calls: %d", calls)
	}
	if got := CachedOverview(a); len(got) != 1 || got[0].Title != a {
		t.Fatal("wrong cached account")
	}
	ExternalConnected = func(string) bool { return false }
	if len(CachedOverview(a)) != 0 {
		t.Fatal("disconnected calendar still shown")
	}
}
