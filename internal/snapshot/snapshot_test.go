package snapshot

import "testing"

// TestRoundTrip verifies publish → broker → mirror over the real go-micro
// store + broker (the memory broker delivers synchronously in-process).
func TestRoundTrip(t *testing.T) {
	s := New("test-card")

	const want = "<div>snapshot</div>"
	s.Publish(want)

	if got := s.Get(); got != want {
		t.Fatalf("Get() = %q, want %q", got, want)
	}
}

// TestEmptyAndNilSafe verifies Get on a fresh/nil snapshot returns "" so callers
// fall back, and Publish("") / nil receiver are no-ops.
func TestEmptyAndNilSafe(t *testing.T) {
	var nilSnap *Snapshot
	if got := nilSnap.Get(); got != "" {
		t.Fatalf("nil Get() = %q, want empty", got)
	}
	nilSnap.Publish("ignored") // must not panic

	s := New("test-empty")
	if got := s.Get(); got != "" {
		t.Fatalf("fresh Get() = %q, want empty", got)
	}
	s.Publish("") // no-op
	if got := s.Get(); got != "" {
		t.Fatalf("after empty Publish Get() = %q, want empty", got)
	}
}
