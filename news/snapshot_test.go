package news

import "testing"

// TestSnapshotRoundTrip verifies the read-plane pattern over the real go-micro
// store + broker: subscribe, publish, and serve from the mirror.
func TestSnapshotRoundTrip(t *testing.T) {
	startSnapshotConsumer()

	const want = "<div>news snapshot</div>"
	publishSnapshot(want)

	if got := snapshot(); got != want {
		t.Fatalf("snapshot mirror = %q, want %q", got, want)
	}
	if got := Headlines(); got != want {
		t.Fatalf("Headlines() = %q, want snapshot %q", got, want)
	}
}

// TestHeadlinesFallback verifies that with no snapshot, Headlines falls back to
// the locally-cached HTML (no regression).
func TestHeadlinesFallback(t *testing.T) {
	snapshotMu.Lock()
	snapshotMirror = ""
	snapshotMu.Unlock()

	mutex.Lock()
	headlinesHtml = "<div>local news fallback</div>"
	mutex.Unlock()

	if got := Headlines(); got != "<div>local news fallback</div>" {
		t.Fatalf("Headlines() fallback = %q, want local HTML", got)
	}
}
