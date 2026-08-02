package social

import (
	"testing"

	"mu/internal/snapshot"
)

// TestCardHTMLServesSnapshot verifies CardHTML serves the broker-fed snapshot
// once one has been published.
func TestCardHTMLServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("social")
	const want = "<div>social snapshot</div>"
	cardSnap.Publish(want)
	if got := CardHTML(); got != want {
		t.Fatalf("CardHTML() = %q, want snapshot %q", got, want)
	}
}

// TestCardHTMLFallback verifies CardHTML falls back to locally-cached HTML when
// no snapshot is available (no regression).
func TestCardHTMLFallback(t *testing.T) {
	cardSnap = nil
	mutex.Lock()
	cardHTML = "<div>local social fallback</div>"
	mutex.Unlock()
	if got := CardHTML(); got != "<div>local social fallback</div>" {
		t.Fatalf("CardHTML() fallback = %q, want local HTML", got)
	}
}
