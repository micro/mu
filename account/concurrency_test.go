package account

// Two requests at once must not lose money.
//
// Every one of these is an ordinary pair of HTTP requests: somebody opening
// /account while their top-up settles, an agent spending while a transfer
// lands, two tool calls from the same token arriving together. The server
// handles each in its own goroutine, so "at the same time" is the normal case
// rather than the unlucky one.
//
// Run these with -race. Without it a lost update still shows up as a wrong
// total, but a torn read of a balance does not show up at all.

import (
	"fmt"
	"sync"
	"testing"

	"mu/internal/quota"
)

// The lost update: reading a balance that does not exist yet, while a credit
// for it lands.
//
// CreditsOf checked the map under a read lock, released it, built an empty
// record, then took the write lock and stored it. A credit arriving in that gap
// created the account first — and then the empty record was written over the
// top of it. The money was gone, from a page load.
//
// Fresh account per attempt, because the window only exists before the record
// does.
func TestReadingANewBalanceDoesNotEraseACreditLandingBesideIt(t *testing.T) {
	const attempts = 200

	for i := 0; i < attempts; i++ {
		id := fmt.Sprintf("race-create-%d", i)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			Balance(id) // the page load
		}()
		go func() {
			defer wg.Done()
			AddCredits(id, 100, quota.OpTopup, nil) //nolint:errcheck
		}()
		wg.Wait()

		if got := Balance(id); got != 100 {
			t.Fatalf("attempt %d: balance is %d, want 100 — a page load overwrote a "+
				"credit that landed while it was reading", i, got)
		}
	}
}

// Concurrent credits all land. Nothing is dropped by two writers racing.
func TestConcurrentCreditsAllLand(t *testing.T) {
	const id = "race-credit"
	const writers = 50

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			AddCredits(id, 1, quota.OpTopup, nil) //nolint:errcheck
		}()
	}
	wg.Wait()

	if got := Balance(id); got != writers {
		t.Errorf("balance is %d, want %d — credits were lost to a concurrent write", got, writers)
	}
}

// And concurrent spends cannot overdraw.
//
// DeductCredits checks the balance and then subtracts. If those are not one
// step, two calls both see enough and both take it: the balance goes negative,
// which is a caller getting something for nothing.
func TestConcurrentSpendsCannotOverdraw(t *testing.T) {
	const id = "race-spend"
	if err := AddCredits(id, 50, quota.OpTopup, nil); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			DeductCredits(id, 1, quota.OpAgentQuery, nil) //nolint:errcheck
		}()
	}
	wg.Wait()

	if got := Balance(id); got < 0 {
		t.Errorf("balance is %d — concurrent spends overdrew the account", got)
	}
}

// A payment arriving twice at once settles once.
//
// The webhook and the return from checkout can land together, which is the
// whole reason both exist. CreditOnce has to decide and write without a gap, or
// both see nothing settled and both credit.
func TestConcurrentSettlementsCreditOnce(t *testing.T) {
	const id = "race-settle"
	before := Balance(id)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			CreditOnce(id, 500, quota.OpTopup, "cs_race_1", nil) //nolint:errcheck
		}()
	}
	wg.Wait()

	if got := Balance(id); got != before+500 {
		t.Errorf("balance is %d, want %d — the same payment settled more than once",
			got, before+500)
	}
}
