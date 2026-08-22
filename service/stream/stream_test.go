package stream

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mu/internal/data"
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

// A restart must not repeat what is already there.
//
// news and video each announce the top of their feed and remember what they
// last said in a package variable, which is empty again after a restart — so a
// redeploy re-announced the current headline and the current video. Five
// restarts in an afternoon put five identical rows on the timeline, which is
// how this was found.
func TestAnEntryAlreadyOnTheTimelineIsNotAddedTwice(t *testing.T) {
	reset(t)

	add(&Entry{Service: "news", Text: "A headline", URL: "https://example.test/a"})
	add(&Entry{Service: "news", Text: "A headline", URL: "https://example.test/a"})
	if got := Recent(10, ""); len(got) != 1 {
		t.Fatalf("the same story is on the timeline %d times", len(got))
	}

	// The link identifies the story, so the same one retitled is still the same
	// one — a feed that rewrites a headline in place must not produce a row per
	// rewrite.
	add(&Entry{Service: "news", Text: "A headline, updated", URL: "https://example.test/a"})
	if got := Recent(10, ""); len(got) != 1 {
		t.Fatalf("a retitled story was added again: %d rows", len(got))
	}

	// A different story is a different row.
	add(&Entry{Service: "news", Text: "Another", URL: "https://example.test/b"})
	if got := Recent(10, ""); len(got) != 2 {
		t.Fatalf("a new story did not reach the timeline: %d rows", len(got))
	}

	// And the same text from a different service is not the same fact.
	add(&Entry{Service: "video", Text: "A headline", URL: "https://example.test/a"})
	if got := Recent(10, ""); len(got) != 3 {
		t.Fatalf("two services announcing one link collapsed into %d rows", len(got))
	}
}

// Mail is not on the timeline, and the rows that are there go.
//
// It was a row: sender, subject, and a link to /inbox — a worse rendering of
// something one click away, a notification on an instance that has none, and
// the one private item on a page of headlines. It was also broken: the
// subscriber read "from", "subject" and "account" off the event, and the mail
// bus migration moved the payload to a single "message" key, so every arrival
// hit the guard and was dropped. That failed closed, which is why nothing
// leaked — but nothing worked either, and the rows still on disk outlived the
// code that wrote them.
func TestMailIsNotOnTheTimeline(t *testing.T) {
	reset(t)

	mu.Lock()
	entries = []*Entry{
		{ID: "1", Service: "mail", Text: "someone@example.com — Invoice",
			URL: "/inbox", Account: "alice", At: time.Now()},
		{ID: "2", Service: "news", Text: "A headline",
			URL: "https://example.test/a", At: time.Now()},
	}
	save()
	entries = nil
	mu.Unlock()

	Load()

	got := Recent(10, "alice")
	if len(got) != 1 {
		t.Fatalf("timeline holds %d entries, want the one that is not mail", len(got))
	}
	if got[0].Service != "news" {
		t.Fatalf("the surviving entry is %q, want news", got[0].Service)
	}

	// And it is gone from disk, not merely hidden — an operator should not have
	// to be told to delete a file.
	var onDisk []*Entry
	if b, err := data.LoadFile("stream.json"); err == nil {
		_ = json.Unmarshal(b, &onDisk)
	}
	for _, e := range onDisk {
		if e.Service == "mail" {
			t.Errorf("a mail row is still in stream.json: %+v", e)
		}
	}
}

// Two accounts are two people. A personal entry has no link, so it keys on its
// text — and "Generated an image: a cat" from two accounts is two facts.
func TestTheSameTextForTwoAccountsIsTwoEntries(t *testing.T) {
	reset(t)
	add(&Entry{Service: "images", Text: "Generated an image: a cat", Account: "alice"})
	add(&Entry{Service: "images", Text: "Generated an image: a cat", Account: "bob"})

	if got := Recent(10, "alice"); len(got) != 1 {
		t.Fatalf("alice sees %d, want her own", len(got))
	}
	if got := Recent(10, "bob"); len(got) != 1 {
		t.Fatalf("bob's entry was dropped as a repeat of alice's")
	}
}
