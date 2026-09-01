package usage

// What one caller is calling.
//
// The store kept three independent tallies — names, users, surfaces — so
// "asim: 400" and "news_list: 900" could both be on the page with nothing
// connecting them. The association was never recorded, so no query could
// recover it and the Callers table was a list of numbers an operator could not
// act on.

import (
	"testing"
	"time"
)

func TestOneCallersOwnBreakdown(t *testing.T) {
	reset(t)

	for i := 0; i < 3; i++ {
		Record("web", "news_list", "asim")
	}
	Record("web", "mail_send", "asim")
	// Somebody else, busy on something else — the breakdown must not pick it up.
	for i := 0; i < 9; i++ {
		Record("api", "blog_create", "henrik")
	}

	rows := TopFor(Minute, 5, "asim", 10)
	if len(rows) != 2 {
		t.Fatalf("asim called two things, got %d rows: %+v", len(rows), rows)
	}
	if rows[0].Key != "news_list" || rows[0].Count != 3 {
		t.Errorf("busiest first: want news_list 3, got %s %d", rows[0].Key, rows[0].Count)
	}
	if rows[1].Key != "mail_send" || rows[1].Count != 1 {
		t.Errorf("want mail_send 1, got %s %d", rows[1].Key, rows[1].Count)
	}

	// The drill-down adds up to the row it was reached from. A breakdown that
	// summed to less than the total it hangs off reads as a bug, which is why
	// this comes from the same store rather than from the ledger.
	var sum int
	for _, r := range rows {
		sum += r.Count
	}
	var total int
	for _, c := range Top(Minute, 5, ByUser, 10) {
		if c.Key == "asim" {
			total = c.Count
		}
	}
	if sum != total {
		t.Errorf("asim's breakdown sums to %d but the Callers table says %d", sum, total)
	}
}

// Nothing for somebody who has not called, and nothing for nobody.
func TestAnUnknownCallerHasNoBreakdown(t *testing.T) {
	reset(t)
	Record("web", "news_list", "asim")

	if rows := TopFor(Minute, 5, "nobody", 10); len(rows) != 0 {
		t.Errorf("an account that called nothing has %d rows", len(rows))
	}
	if rows := TopFor(Minute, 5, "", 10); len(rows) != 0 {
		t.Errorf("the empty caller has %d rows", len(rows))
	}
}

// A caller whose name is a prefix of another's is not folded into it.
//
// The key is one string holding two values, so the split has to be on the
// separator and not on a prefix match: "asim" and "asimov" would otherwise
// share a breakdown, with asimov's tools appearing under asim with their names
// mangled.
func TestOneCallerIsNotAPrefixOfAnother(t *testing.T) {
	reset(t)
	Record("web", "news_list", "asim")
	Record("web", "blog_create", "asimov")

	rows := TopFor(Minute, 5, "asim", 10)
	if len(rows) != 1 || rows[0].Key != "news_list" {
		t.Errorf("asim's breakdown picked up asimov's calls: %+v", rows)
	}
}

// A bucket written before Pairs existed has a nil map, and the first request
// after that deploy must not panic on it.
func TestAStoreFromBeforeThisSurvivesARequest(t *testing.T) {
	reset(t)

	mu.Lock()
	b := rings.Minute.current(now().UTC())
	b.Pairs = nil // as JSON with no "pairs" key unmarshals
	mu.Unlock()

	Record("web", "news_list", "asim") // must not panic

	// And the other tallies still work, so an upgraded instance is not blank.
	if got := Top(Minute, 5, ByUser, 10); len(got) == 0 {
		t.Error("the caller tally stopped counting when Pairs was nil")
	}
}

// reset empties the store between tests.
func reset(t *testing.T) {
	t.Helper()
	mu.Lock()
	rings = newStore()
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		rings = newStore()
		mu.Unlock()
	})
	_ = time.Now
}
