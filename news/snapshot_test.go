package news

import (
	"testing"

	"mu/internal/snapshot"
)

// TestHeadlinesServesSnapshot verifies Headlines serves the broker-fed snapshot
// once one has been published.
func TestHeadlinesServesSnapshot(t *testing.T) {
	cardSnap = snapshot.New("news")
	const want = "<div>news snapshot</div>"
	cardSnap.Publish(want)
	if got := Headlines(); got != want {
		t.Fatalf("Headlines() = %q, want snapshot %q", got, want)
	}
}

// TestHeadlinesFallback verifies Headlines falls back to locally-cached HTML
// when no snapshot is available (no regression).
func TestHeadlinesFallback(t *testing.T) {
	cardSnap = nil
	mutex.Lock()
	headlinesHtml = "<div>local news fallback</div>"
	mutex.Unlock()
	if got := Headlines(); got != "<div>local news fallback</div>" {
		t.Fatalf("Headlines() fallback = %q, want local HTML", got)
	}
}
