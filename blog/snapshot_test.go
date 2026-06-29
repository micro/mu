package blog

import (
	"testing"

	"mu/internal/snapshot"
)

// TestPreviewServesSnapshot verifies Preview serves the broker-fed snapshot once
// one has been published.
func TestPreviewServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("blog")
	const want = "<div>blog snapshot</div>"
	cardSnap.Publish(want)
	if got := Preview(); got != want {
		t.Fatalf("Preview() = %q, want snapshot %q", got, want)
	}
}

// TestPreviewFallback verifies Preview falls back to locally-cached HTML when no
// snapshot is available (no regression).
func TestPreviewFallback(t *testing.T) {
	cardSnap = nil
	mutex.Lock()
	postsPreviewHtml = "<div>local blog fallback</div>"
	mutex.Unlock()
	if got := Preview(); got != "<div>local blog fallback</div>" {
		t.Fatalf("Preview() fallback = %q, want local HTML", got)
	}
}
