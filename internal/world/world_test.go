package world

import (
	"strings"
	"testing"
	"time"

	"mu/internal/event"
)

func TestWhatHappenedIsKept(t *testing.T) {
	Forget()
	Watch()

	event.Announce("news", "Something happened", "https://example.test/a", "")
	event.Announce("video", "A video appeared", "", "")

	got := waitFor(t, 2)
	// Newest first, which is the order a reader wants and the order a prompt
	// should carry: the last thing that happened is the one most likely to be
	// what somebody is asking about.
	if got[0].Service != "video" {
		t.Errorf("newest is %q, want the video that came second", got[0].Service)
	}
	if got[1].Text != "Something happened" {
		t.Errorf("second is %q", got[1].Text)
	}
}

// Somebody's own is not the world's.
//
// event.Announce takes an account for exactly this distinction — a message that
// arrived, an image somebody generated — and its own comment records how a
// public timeline came to be carrying people's mail. This structure is read by
// every agent on the instance, so a personal fact is refused at the door rather
// than filtered on the way out.
func TestPersonalActivityNeverEntersTheWorld(t *testing.T) {
	Forget()
	Watch()

	event.Announce("images", "Generated an image: a private prompt", "/images/1", "asim")
	event.Announce("news", "A public headline", "", "")

	got := waitFor(t, 1)
	for _, c := range got {
		if strings.Contains(c.Text, "private prompt") {
			t.Fatalf("somebody's own activity is in the world view: %#v", c)
		}
	}
	if len(got) != 1 || got[0].Service != "news" {
		t.Errorf("the public fact did not survive: %#v", got)
	}
}

// A caller sees only what it asked about.
func TestScopeFiltersTheChanges(t *testing.T) {
	Forget()
	Watch()

	event.Announce("news", "Headline", "", "")
	event.Announce("video", "Clip", "", "")
	waitFor(t, 2)

	if got := Lately("news"); len(got) != 1 || got[0].Service != "news" {
		t.Errorf("scoped to news and got %#v", got)
	}
	if got := Lately("shell"); len(got) != 0 {
		t.Errorf("a scope with no changes returned %#v", got)
	}
	if got := Lately(); len(got) != 2 {
		t.Errorf("no scope should be the whole world, got %d", len(got))
	}
}

// What is too old stops being news, and what is too much is dropped oldest
// first.
func TestTheRecordIsBounded(t *testing.T) {
	Forget()

	mu.Lock()
	// Written directly rather than announced: the point is what trimming does,
	// and a test that waited for a hundred events through the bus would be
	// testing the bus.
	for i := 0; i < remembered+10; i++ {
		changes = append(changes, Change{At: time.Now().UTC(), Service: "news", Text: "x"})
	}
	changes = append(changes, Change{
		At: time.Now().UTC().Add(-forgotten - time.Hour), Service: "news", Text: "yesterday",
	})
	trimLocked()
	held := len(changes)
	mu.Unlock()

	if held > remembered {
		t.Errorf("holding %d changes, want at most %d", held, remembered)
	}
	for _, c := range Lately() {
		if c.Text == "yesterday" {
			t.Error("a change older than the window is still being reported as news")
		}
	}
}

// Watching twice is watching once.
func TestWatchIsIdempotent(t *testing.T) {
	Forget()
	Watch()
	Watch()

	event.Announce("news", "Once", "", "")
	got := waitFor(t, 1)
	if len(got) != 1 {
		t.Errorf("one announcement was recorded %d times", len(got))
	}
}

// waitFor gives the bus a moment to deliver, and returns what arrived.
func waitFor(t *testing.T, n int) []Change {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := Lately()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited for %d changes and %d arrived: %#v", n, len(got), got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
