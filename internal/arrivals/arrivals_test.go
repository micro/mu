package arrivals

// What came in today.
//
// Three things have to hold. It counts today and not the archive; it counts
// arrivals and not every kind of row the services write; and it works the
// answer out again when something lands, without working it out on every ask.

import (
	"testing"
	"time"

	"mu/internal/data"
)

// put indexes one row, stamped with when the thing happened.
func put(t *testing.T, id, kind, title string, at time.Time) {
	t.Helper()
	data.StartIndexing()
	data.Index(id, kind, title, "body of "+id, map[string]interface{}{
		"posted_at": at.Format(time.RFC3339),
	})
	settle(t, id)
	forget()
}

// settle waits for a queued entry to land, because indexing is done by
// background workers and a test that reached past them would pass against code
// that never indexes anything.
func settle(t *testing.T, ids ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for _, id := range ids {
		for data.ByID(id) == nil {
			if time.Now().After(deadline) {
				t.Fatalf("%s was never indexed", id)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// clean empties the index so one test's rows are not another's answer.
func clean(t *testing.T) {
	t.Helper()
	for _, k := range []string{"news", "video", "social", "market", "reminder", "post"} {
		for _, e := range data.ByType(k, 500) {
			data.Unindex(e.ID)
		}
	}
	forget()
}

func TestAnEmptyArchiveSaysNothing(t *testing.T) {
	clean(t)

	if day := Today(); day.Any() {
		t.Errorf("an empty archive counted %d things: %v", day.Total(), day.Counts)
	}
}

// The count is of today, so yesterday's news is not today's.
func TestOnlyTodayCounts(t *testing.T) {
	clean(t)

	now := time.Now()
	put(t, "a-today", "news", "Rates held at 4%", now.Add(-time.Hour))
	put(t, "a-yesterday", "news", "Rates cut in March", now.AddDate(0, 0, -1))
	put(t, "a-lastweek", "news", "An old story", now.AddDate(0, 0, -7))

	day := Today()
	if got := day.Counts["news"]; got != 1 {
		t.Errorf("counted %d stories today, want 1: %v", got, day.Counts)
	}
	if day.Newest != "Rates held at 4%" {
		t.Errorf("the newest is %q", day.Newest)
	}
}

// A row indexed today about something old did not happen today. This is the
// difference between the two stamps, and it is the one that would quietly
// inflate the number every time a service re-reads its feed.
func TestAnOldStoryIndexedNowIsNotNews(t *testing.T) {
	clean(t)

	put(t, "a-stale", "news", "Something from March", time.Now().AddDate(0, 0, -60))

	if day := Today(); day.Any() {
		t.Errorf("a two month old article counted as today: %v", day.Counts)
	}
}

// Every kind is counted in its own words, and the words are not the type name.
func TestEachKindIsCountedInItsOwnWords(t *testing.T) {
	clean(t)

	now := time.Now()
	put(t, "a-n1", "news", "One story", now.Add(-3*time.Hour))
	put(t, "a-n2", "news", "Two stories", now.Add(-2*time.Hour))
	put(t, "a-v1", "video", "A video", now.Add(-time.Hour))
	put(t, "a-s1", "social", "someone", now.Add(-30*time.Minute))

	day := Today()
	for kind, want := range map[string]int{"news": 2, "video": 1, "social": 1} {
		if got := day.Counts[kind]; got != want {
			t.Errorf("%s counted %d, want %d: %v", kind, got, want, day.Counts)
		}
	}
	if day.Total() != 4 {
		t.Errorf("total is %d, want 4", day.Total())
	}

	// Every kind has words to be counted in. The type name is what the
	// indexing service called the row and is not always a noun a person would
	// use: rows are typed "social" and nobody has received nine socials.
	for _, k := range Kinds {
		if k.One == "" || k.Many == "" {
			t.Errorf("%s has no words to be counted in (%q/%q)", k.Type, k.One, k.Many)
		}
		if k.Type == "social" && k.One == "social" {
			t.Error(`social is counted as "socials"`)
		}
	}
}

// Prices are levels, not arrivals, and reminders land hourly by construction.
// Counting either would put a number on the line that changes on a timer and
// means nothing.
func TestPricesAndRemindersAreNotArrivals(t *testing.T) {
	clean(t)

	now := time.Now()
	put(t, "a-mkt", "market", "AAPL", now)
	put(t, "a-rem", "reminder", "Al-Baqarah 2:255", now)
	put(t, "a-post", "post", "The daily digest", now)

	if day := Today(); day.Any() {
		t.Errorf("a price, a reminder and a blog post counted as arrivals: %v", day.Counts)
	}
}

// The newest is the most recent thing that happened, across kinds — not the
// most recent of whichever kind is read first.
func TestTheNewestIsTheNewestOfAnything(t *testing.T) {
	clean(t)

	now := time.Now()
	put(t, "a-old-news", "news", "An earlier story", now.Add(-4*time.Hour))
	put(t, "a-new-video", "video", "The latest video", now.Add(-10*time.Minute))

	if got := Today().Newest; got != "The latest video" {
		t.Errorf("the newest is %q, want the video", got)
	}
}

// The answer is cached, and the cache is dropped when something lands.
func TestTheCountIsNotRedoneUntilSomethingArrives(t *testing.T) {
	clean(t)

	now := time.Now()
	data.StartIndexing()
	data.Index("a-c1", "news", "First", "body", map[string]interface{}{
		"posted_at": now.Add(-time.Hour).Format(time.RFC3339),
	})
	settle(t, "a-c1")
	forget()

	first := Today()
	if first.Counts["news"] != 1 {
		t.Fatalf("counted %v", first.Counts)
	}

	// Asked again with nothing new, the same answer comes back without being
	// worked out again — same At stamp, because count() sets a fresh one.
	again := Today()
	if !again.At.Equal(first.At) {
		t.Errorf("the count was redone with nothing new: %v then %v", first.At, again.At)
	}

	data.Index("a-c2", "news", "Second", "body", map[string]interface{}{
		"posted_at": now.Add(-30 * time.Minute).Format(time.RFC3339),
	})
	settle(t, "a-c2")

	after := Today()
	if after.Counts["news"] != 2 {
		t.Errorf("an arrival did not move the count: %v", after.Counts)
	}
	if after.At.Equal(first.At) {
		t.Error("an arrival did not invalidate the cached answer")
	}
}

// A day rolls over even when nothing arrives to force a recount, or the front
// page reads "12 stories today" all through tomorrow morning.
func TestTheCacheExpiresWithTheDay(t *testing.T) {
	clean(t)

	put(t, "a-d1", "news", "Today's story", time.Now().Add(-time.Hour))
	day := Today()
	if !day.Any() {
		t.Fatal("nothing counted")
	}

	// Pretend the cached answer was worked out yesterday.
	mu.Lock()
	cached.Day = cached.Day.AddDate(0, 0, -1)
	stale := cached.At
	mu.Unlock()

	if got := Today(); got.At.Equal(stale) {
		t.Error("yesterday's count was served today")
	}
	if got := Today(); !got.Day.Equal(midnight(time.Now())) {
		t.Errorf("the recount is dated %v, want today", got.Day)
	}
}

// A social row's title is whoever posted it, not what they said, so it cannot
// be read out as the newest thing that happened. "The newest “someone”" was
// what the first version of this said.
func TestOnlyAHeadlineIsReadOutAsTheNewest(t *testing.T) {
	clean(t)

	now := time.Now()
	put(t, "a-h-news", "news", "Rates held at 4%", now.Add(-2*time.Hour))
	put(t, "a-h-social", "social", "someone", now.Add(-5*time.Minute))

	day := Today()
	if day.Total() != 2 {
		t.Fatalf("counted %v", day.Counts)
	}
	if day.Newest != "Rates held at 4%" {
		t.Errorf("the newest is %q — a social row's title is its author", day.Newest)
	}

	// Posts alone leave nothing to name, which is better than naming a person
	// as though they were an event.
	clean(t)
	put(t, "a-h-only", "social", "someone else", now.Add(-5*time.Minute))
	if got := Today(); got.Newest != "" {
		t.Errorf("a lone post was named %q", got.Newest)
	} else if !got.Any() {
		t.Error("the post was not counted")
	}
}

// Rows are removed as well as added — an account deleted, a superseded store
// swept — and the gate only sees things appear. Without an age on the cached
// answer the count could only ever go up.
func TestARemovedRowStopsBeingCounted(t *testing.T) {
	clean(t)

	put(t, "a-gone", "news", "A story that gets deleted", time.Now().Add(-time.Hour))
	if Today().Counts["news"] != 1 {
		t.Fatal("the story was not counted in the first place")
	}

	data.Unindex("a-gone")

	// Nothing has arrived, so only the age can force the recount.
	mu.Lock()
	cached.At = cached.At.Add(-stale - time.Second)
	mu.Unlock()

	if day := Today(); day.Any() {
		t.Errorf("a deleted story is still counted: %v", day.Counts)
	}
}

func TestMidnightIsTheStartOfTheDay(t *testing.T) {
	at := time.Date(2026, 8, 30, 14, 33, 9, 500, time.UTC)
	want := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	if got := midnight(at); !got.Equal(want) {
		t.Errorf("midnight(%v) = %v, want %v", at, got, want)
	}
}
