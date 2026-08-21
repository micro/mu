package stream

import (
	"fmt"
	"strings"
	"testing"
)

func reset(t *testing.T) {
	t.Helper()
	mu.Lock()
	entries = nil
	// And on disk. Load reads stream.json back, so a test leaving entries
	// there is a test the next one inherits.
	save()
	mu.Unlock()
}

func TestTheTimelineTrimsToMaxEntries(t *testing.T) {
	reset(t)

	for i := range MaxEntries + 5 {
		add(&Entry{Service: "news", Text: fmt.Sprintf("entry-%03d", i)})
	}

	got := Recent(MaxEntries+10, "")
	if len(got) != MaxEntries {
		t.Fatalf("Recent returned %d entries, want %d", len(got), MaxEntries)
	}
	if got[0].Text != "entry-504" {
		t.Fatalf("newest = %q, want entry-504", got[0].Text)
	}
	if got[len(got)-1].Text != "entry-005" {
		t.Fatalf("oldest retained = %q, want entry-005", got[len(got)-1].Text)
	}
}

func TestClearEmptiesTheTimeline(t *testing.T) {
	reset(t)
	add(&Entry{Service: "news", Text: "before clear"})
	Clear()
	add(&Entry{Service: "news", Text: "after clear"})

	got := Recent(10, "")
	if len(got) != 1 || got[0].Text != "after clear" {
		t.Fatalf("after clear the timeline holds %v, want the one entry", got)
	}
}

// An announced fact arrives as a map off the bus. The three fields that decide
// what a reader sees — where it came from, what it says, and whose it is —
// have to survive that trip, because there is no type on the other side to
// catch a renamed key.
func TestAnAnnouncedFactBecomesAnEntry(t *testing.T) {
	e := fromEvent(map[string]any{
		"service": "blog",
		"text":    "New post: Hello",
		"url":     "/blog/post?id=1",
		"account": "alice",
	})
	if e.Service != "blog" || e.Text != "New post: Hello" ||
		e.URL != "/blog/post?id=1" || e.Account != "alice" {
		t.Fatalf("announced fact flattened to %+v", e)
	}
}

// A fact with nothing to say is dropped rather than stored as a blank row.
func TestAnEmptyFactIsNotStored(t *testing.T) {
	reset(t)
	add(fromEvent(map[string]any{"service": "blog"}))
	add(fromEvent(map[string]any{"text": "orphan"}))
	if got := Recent(10, ""); len(got) != 0 {
		t.Fatalf("timeline holds %v, want nothing", got)
	}
}

// An entry rendered before its service exists — or after it has gone — still
// says where it came from. The label is read off the registry, and a timeline
// outlives the thing that wrote to it.
func TestAnUnknownServiceStillNamesItself(t *testing.T) {
	out := renderEntry(&Entry{Service: "gone", Text: "something happened"})
	if !strings.Contains(out, "gone") {
		t.Errorf("rendered entry does not name its source:\n%s", out)
	}
}
