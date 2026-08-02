package markets

import (
	"testing"

	"mu/internal/snapshot"
)

// TestMarketsHTMLServesSnapshot verifies MarketsHTML serves the broker-fed
// snapshot once one has been published.
func TestMarketsHTMLServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("markets")
	const want = "<div>markets snapshot</div>"
	cardSnap.Publish(want)
	if got := MarketsHTML(); got != want {
		t.Fatalf("MarketsHTML() = %q, want snapshot %q", got, want)
	}
}

// TestMarketsHTMLFallback verifies MarketsHTML falls back to locally-generated
// HTML when no snapshot is available (no regression).
func TestMarketsHTMLFallback(t *testing.T) {
	cardSnap = nil // simulate "no snapshot channel / nothing published"
	marketsMutex.Lock()
	marketsHTML = "<div>local fallback</div>"
	marketsMutex.Unlock()
	if got := MarketsHTML(); got != "<div>local fallback</div>" {
		t.Fatalf("MarketsHTML() fallback = %q, want local HTML", got)
	}
}
