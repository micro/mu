package video

import (
	"testing"

	"mu/internal/snapshot"
)

// TestLatestServesSnapshot verifies Latest serves the broker-fed snapshot once
// one has been published.
func TestLatestServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("video")
	const want = "<div>video snapshot</div>"
	cardSnap.Publish(want)
	if got := Latest(); got != want {
		t.Fatalf("Latest() = %q, want snapshot %q", got, want)
	}
}

// TestLatestFallback verifies Latest falls back to locally-cached HTML when no
// snapshot is available (no regression).
func TestLatestFallback(t *testing.T) {
	cardSnap = nil
	mutex.Lock()
	latestHtml = "<div>local video fallback</div>"
	mutex.Unlock()
	if got := Latest(); got != "<div>local video fallback</div>" {
		t.Fatalf("Latest() fallback = %q, want local HTML", got)
	}
}
