package data

// What one search costs, and the two things that made it cost too much.
//
// Measured on a copy of a real index grown to 126,208 rows, before any of this:
//
//	Kinds() GROUP BY        148ms   — on every /archive request, for five chips
//	FTS5                    0.4ms   — the part that works, flat in table size
//	LIKE fallback           669ms   — fired on almost every search
//
// 817ms of database work per search, of which 0.4ms was the index doing its
// job. None of it shows at a thousand rows, which is why it shipped.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// The caller's limit is the caller's limit.
//
// Both phases said LIMIT 50 as a literal and the argument was ignored, so an
// agent asking for three read fifty rows — an article body each — and threw
// forty-seven away.
func TestSearchReturnsWhatWasAskedFor(t *testing.T) {
	os.Setenv("HOME", t.TempDir())
	UseSQLite = false
	ClearIndex()

	for i := 0; i < 40; i++ {
		processIndexWork(IndexWork{
			ID: fmt.Sprintf("n%d", i), Type: "news",
			Title:   fmt.Sprintf("Bitcoin story %d", i),
			Content: "a story about bitcoin and markets",
		})
	}

	for _, want := range []int{1, 3, 10} {
		if got := len(Search("bitcoin", want)); got > want {
			t.Errorf("asked for %d results, got %d", want, got)
		}
	}
	// And asking for nothing in particular does not mean everything.
	if got := len(Search("bitcoin", 0)); got > maxFetch {
		t.Errorf("an unbounded search returned %d rows", got)
	}
}

// The scan only runs when the index found nothing.
//
// The condition was `len(allEntries) < limit`, which fires on nearly every
// search — you skip the scan only by getting a full page of FTS hits, so the
// better the index works the more likely you still pay for it. And it cannot
// be made cheap: a leading wildcard means no index applies, so the LIKE reads
// every public row's content and sorts the survivors in a temp b-tree.
//
// Read from source, because the cost is in a branch rather than in a result:
// both conditions return the same rows, and only one of them reads the corpus
// to do it.
func TestTheScanOnlyRunsWhenTheIndexFoundNothing(t *testing.T) {
	src, err := os.ReadFile("sqlite.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	if !strings.Contains(body, "if len(allEntries) == 0 {") {
		t.Error("the LIKE fallback no longer waits for the index to come back empty")
	}
	if strings.Contains(body, "if len(allEntries) < limit {") {
		t.Error("the LIKE fallback fires whenever FTS returned less than a full " +
			"page, which is almost every search — measured at 669ms on 126k rows")
	}
	// And neither phase reads an unbounded amount however large a limit is.
	if strings.Contains(body, "LIMIT 50`") {
		t.Error("a phase still hardcodes LIMIT 50, ignoring what the caller asked for")
	}
}

// The chip counts are cached, because they are a whole-table question asked on
// every page load and nobody navigates by whether it says 658 or 659.
func TestTheKindCountsAreCached(t *testing.T) {
	os.Setenv("HOME", t.TempDir())
	UseSQLite = false
	ClearIndex()
	InvalidateKinds()

	processIndexWork(IndexWork{ID: "a", Type: "news", Title: "One", Content: "one"})
	first := Kinds()
	if len(first) != 1 || first[0].Count != 1 {
		t.Fatalf("counts wrong to start with: %+v", first)
	}

	// A second entry inside the window does not re-run the grouping, so the
	// answer is the one already computed.
	processIndexWork(IndexWork{ID: "b", Type: "news", Title: "Two", Content: "two"})
	if again := Kinds(); again[0].Count != 1 {
		t.Errorf("the grouping ran again inside the cache window: %+v", again)
	}

	// And the caller holds its own copy: a reader that sorted or truncated the
	// returned slice would otherwise corrupt what everybody else gets.
	got := Kinds()
	got[0].Count = 999
	if after := Kinds(); after[0].Count == 999 {
		t.Error("Kinds hands out the cached slice itself, so a caller can edit it")
	}

	// Invalidating makes it exact again, for a caller that has just changed
	// enough to care.
	InvalidateKinds()
	if fresh := Kinds(); fresh[0].Count != 2 {
		t.Errorf("after invalidating, the count is still stale: %+v", fresh)
	}
}

// The chips count what is on the page, not what is in the archive.
//
// They were always the whole-archive breakdown, so searching "bitcoin" put
// "market 658" beside a list of bitcoin results and invited the obvious
// reading. The number was true about the archive and false about everything
// else on the screen.
func TestTheKindCountsForAQueryCountTheQuery(t *testing.T) {
	os.Setenv("HOME", t.TempDir())
	UseSQLite = false
	ClearIndex()
	InvalidateKinds()

	for i := 0; i < 5; i++ {
		processIndexWork(IndexWork{ID: fmt.Sprintf("n%d", i), Type: "news",
			Title: "Bitcoin story", Content: "bitcoin"})
	}
	for i := 0; i < 20; i++ {
		processIndexWork(IndexWork{ID: fmt.Sprintf("m%d", i), Type: "market",
			Title: "Gold close", Content: "gold bullion"})
	}

	whole := Kinds()
	byName := map[string]int{}
	for _, k := range whole {
		byName[k.Name] = k.Count
	}
	if byName["market"] != 20 || byName["news"] != 5 {
		t.Fatalf("the archive-wide counts are wrong to start with: %+v", whole)
	}

	got := map[string]int{}
	for _, k := range KindsMatching("bitcoin") {
		got[k.Name] = k.Count
	}
	if got["news"] != 5 {
		t.Errorf("a bitcoin search counts %d news, want 5: %+v", got["news"], got)
	}
	if got["market"] != 0 {
		t.Errorf("a bitcoin search claims %d market results, and there are none", got["market"])
	}
}

// A row's time is when the thing happened, not when this instance wrote it
// down. On a fresh install those are all the same moment, so every row read
// "2 minutes ago" — an article from last March and a video from 2023 included.
func TestARowIsStampedWhenItHappened(t *testing.T) {
	last := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	withPosted := &IndexEntry{
		ID: "a", IndexedAt: time.Now(),
		Metadata: map[string]any{"posted_at": last.Format(time.RFC3339)},
	}
	if got := PostedAt(withPosted); !got.Equal(last) {
		t.Errorf("PostedAt = %v, want the time in the metadata (%v)", got, last)
	}

	// And where the source recorded nothing, the index time is all there is —
	// which is the honest answer rather than a wrong one.
	indexed := time.Now().Add(-time.Hour)
	if got := PostedAt(&IndexEntry{ID: "b", IndexedAt: indexed}); !got.Equal(indexed) {
		t.Errorf("with no posted_at, PostedAt = %v, want the index time", got)
	}
}
