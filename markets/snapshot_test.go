package markets

import "testing"

// TestSnapshotRoundTrip verifies the read-plane pattern end to end over the real
// go-micro store + broker: subscribing, publishing a snapshot, and serving it
// from the mirror. The memory broker delivers synchronously in-process, so the
// mirror is set by the time Publish returns.
func TestSnapshotRoundTrip(t *testing.T) {
	startSnapshotConsumer()

	const want = "<div>markets snapshot</div>"
	publishSnapshot(want)

	if got := snapshot(); got != want {
		t.Fatalf("snapshot mirror = %q, want %q", got, want)
	}
	// MarketsHTML must serve the mirror once a snapshot has arrived.
	if got := MarketsHTML(); got != want {
		t.Fatalf("MarketsHTML() = %q, want snapshot %q", got, want)
	}
}

// TestMarketsHTMLFallback verifies that with no snapshot, MarketsHTML falls back
// to the locally-generated HTML (no regression).
func TestMarketsHTMLFallback(t *testing.T) {
	// Reset mirror to simulate "no snapshot yet".
	snapshotMu.Lock()
	snapshotMirror = ""
	snapshotMu.Unlock()

	marketsMutex.Lock()
	marketsHTML = "<div>local fallback</div>"
	marketsMutex.Unlock()

	if got := MarketsHTML(); got != "<div>local fallback</div>" {
		t.Fatalf("MarketsHTML() fallback = %q, want local HTML", got)
	}
}
