package thread

// The store is written by every client and read by every page, and it has one
// lock.
//
// Two things went wrong under that. The flusher marshalled the live structs
// after releasing the lock, so a save read every field of every message while
// Add was writing to them — a data race, and the kind that corrupts the file it
// is writing rather than crashing somewhere you would see it. And every
// question about one account walked every thread on the instance, so a busy
// instance scanned the whole record to draw one sidebar with every writer
// queued behind it.

import (
	"fmt"
	"sync"
	"testing"
)

// Run with -race. Without it this passes whatever the code does.
func TestSavingDoesNotRaceWithWriting(t *testing.T) {
	reset(t)

	th := Open("asim", "web", "k")
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			Add(Message{Thread: th.ID, Account: "asim", Text: fmt.Sprintf("m%d", i)})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			dirty.Store(true)
			Flush()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			List("asim", 0)
			Messages("asim", th.ID, 5)
			Search("asim", "m1", "", 5)
		}
	}()
	wg.Wait()
}

// One account's questions do not walk another account's data.
//
// Read off the index rather than timed, because a timing assertion on a shared
// runner is a flake. The claim is structural: the threads reachable for an
// account are exactly that account's, so the scans are bounded by what it owns.
func TestAnAccountIsIndexedByItself(t *testing.T) {
	reset(t)

	for i := 0; i < 50; i++ {
		th := Open("noisy", "web", fmt.Sprintf("k%d", i))
		Add(Message{Thread: th.ID, Account: "noisy", Text: "chatter"})
	}
	mine := Open("quiet", "web", "k0")
	Add(Message{Thread: mine.ID, Account: "quiet", Text: "one thing"})

	mu.RLock()
	n := len(owned["quiet"])
	total := len(threads)
	mu.RUnlock()
	if n != 1 {
		t.Errorf("the quiet account owns %d threads, want 1", n)
	}
	if total != 51 {
		t.Fatalf("the store holds %d threads, want 51 — this test is not measuring what it thinks", total)
	}
	if got := len(List("quiet", 0)); got != 1 {
		t.Errorf("listing the quiet account returned %d", got)
	}

	// And deleting keeps the index honest, which is what stops a deleted
	// conversation coming back the next time anything walks the account.
	Delete("quiet", mine.ID)
	mu.RLock()
	_, still := owned["quiet"]
	mu.RUnlock()
	if still {
		t.Error("an account with no threads left is still in the index")
	}
	Forget("noisy")
	mu.RLock()
	left := len(threads)
	mu.RUnlock()
	if left != 0 {
		t.Errorf("%d threads survived deleting both accounts", left)
	}
}
