package memory

import (
	"strings"
	"testing"
)

func resetStore(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	mu.Lock()
	store = map[string][]*Entry{}
	mu.Unlock()
}

func TestSetGetAndUpdateNormalizesInput(t *testing.T) {
	resetStore(t)

	Set("user-1", "  Favorite Color  ", "  blue  ")
	Set("user-1", "favorite color", " green ")
	Set("user-1", "ignored", "   ")
	Set("user-1", "   ", "ignored")

	if got := Get("user-1", "FAVORITE COLOR"); got != "green" {
		t.Fatalf("Get() = %q, want %q", got, "green")
	}
	if got := All("user-1"); len(got) != 1 {
		t.Fatalf("All() returned %d entries, want 1", len(got))
	}
}

func TestAllReturnsIndependentEntries(t *testing.T) {
	resetStore(t)

	Set("user-1", "name", "Ada")
	entries := All("user-1")
	if len(entries) != 1 {
		t.Fatalf("All() returned %d entries, want 1", len(entries))
	}

	entries[0].Value = "Grace"

	if got := Get("user-1", "name"); got != "Ada" {
		t.Fatalf("mutating All() result changed stored value to %q", got)
	}
}

func TestSetCapsEntriesPerUserKeepsNewest(t *testing.T) {
	resetStore(t)

	for i := 0; i < MaxEntriesPerUser+5; i++ {
		Set("user-1", string(rune('a'+i)), "value")
	}

	entries := All("user-1")
	if len(entries) != MaxEntriesPerUser {
		t.Fatalf("All() returned %d entries, want %d", len(entries), MaxEntriesPerUser)
	}
	if got := Get("user-1", "a"); got != "" {
		t.Fatalf("oldest entry was not evicted, got %q", got)
	}
}

func TestForScopedContextIncludesGlobalAndMatchingScope(t *testing.T) {
	resetStore(t)

	Set("user-1", "timezone", "UTC")
	Set("user-1", "news:topic", "AI")
	Set("user-1", "weather:units", "metric")

	got := ForScopedContext("user-1", "news")
	for _, want := range []string{"- timezone: UTC\n", "- topic: AI\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ForScopedContext() = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "weather") || strings.Contains(got, "metric") {
		t.Fatalf("ForScopedContext() = %q, included unrelated scope", got)
	}
}
