package notes

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

	Add("user-1", "  Favorite Color  ", "  blue  ")
	Add("user-1", "favorite color", " green ")
	Add("user-1", "ignored", "   ")
	Add("user-1", "   ", "ignored")

	if got := Get("user-1", "FAVORITE COLOR"); got != "green" {
		t.Fatalf("Get() = %q, want %q", got, "green")
	}
	if got := All("user-1"); len(got) != 1 {
		t.Fatalf("All() returned %d entries, want 1", len(got))
	}
}

func TestAllReturnsIndependentEntries(t *testing.T) {
	resetStore(t)

	Add("user-1", "name", "Ada")
	entries := All("user-1")
	if len(entries) != 1 {
		t.Fatalf("All() returned %d entries, want 1", len(entries))
	}

	entries[0].Text = "Grace"

	if got := Get("user-1", "name"); got != "Ada" {
		t.Fatalf("mutating All() result changed stored value to %q", got)
	}
}

func TestSetCapsEntriesPerUserKeepsNewest(t *testing.T) {
	resetStore(t)

	for i := 0; i < MaxPerUser+5; i++ {
		Add("user-1", string(rune('a'+i)), "value")
	}

	entries := All("user-1")
	if len(entries) != MaxPerUser {
		t.Fatalf("All() returned %d entries, want %d", len(entries), MaxPerUser)
	}
	if got := Get("user-1", "a"); got != "" {
		t.Fatalf("oldest entry was not evicted, got %q", got)
	}
}

func TestForScopedContextIncludesGlobalAndMatchingScope(t *testing.T) {
	resetStore(t)

	Add("user-1", "timezone", "UTC")
	Add("user-1", "news:topic", "AI")
	Add("user-1", "weather:units", "metric")

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

func TestDeleteAndClearRemoveStoredEntries(t *testing.T) {
	resetStore(t)

	Add("user-1", "timezone", "UTC")
	Add("user-1", "theme", "dark")
	Add("user-2", "timezone", "PST")

	Delete("user-1", "TIMEZONE")

	if got := Get("user-1", "timezone"); got != "" {
		t.Fatalf("Delete() left removed entry value %q", got)
	}
	if got := Get("user-1", "theme"); got != "dark" {
		t.Fatalf("Delete() removed unrelated entry, got %q", got)
	}
	if got := Get("user-2", "timezone"); got != "PST" {
		t.Fatalf("Delete() affected another user, got %q", got)
	}

	Clear("user-1")

	if got := All("user-1"); len(got) != 0 {
		t.Fatalf("Clear() left %d entries, want 0", len(got))
	}
	if got := Get("user-2", "timezone"); got != "PST" {
		t.Fatalf("Clear() affected another user, got %q", got)
	}
}
