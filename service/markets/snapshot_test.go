package markets

import (
	"testing"

	"mu/internal/snapshot"
)

// TestMarketsHTMLServesSnapshot verifies HTML serves the broker-fed
// snapshot once one has been published.
func TestMarketsHTMLServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("markets")
	const want = "<div>markets snapshot</div>"
	cardSnap.Publish(want)
	if got := HTML(); got != want {
		t.Fatalf("HTML() = %q, want snapshot %q", got, want)
	}
}

// TestMarketsHTMLFallback verifies HTML falls back to locally-generated
// HTML when no snapshot is available (no regression).
func TestMarketsHTMLFallback(t *testing.T) {
	cardSnap = nil // simulate "no snapshot channel / nothing published"
	marketsMutex.Lock()
	marketsHTML = "<div>local fallback</div>"
	marketsMutex.Unlock()
	if got := HTML(); got != "<div>local fallback</div>" {
		t.Fatalf("HTML() fallback = %q, want local HTML", got)
	}
}
