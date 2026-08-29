package thread

import (
	"os"
	"path/filepath"
	"testing"
)

// A fresh HOME is a fresh record.
//
// The rule this holds: whichever ~/.mu HOME points at when somebody asks for a
// conversation is the record they get. It sounds like it could not be otherwise
// and it was otherwise for a long time — the record was read in init(), which
// runs before a test, a CLI flag or anything else can say where HOME is, so the
// answer was fixed at process start and no later caller could change it.
//
// What that cost: every test in this repo that sets HOME to a temp directory
// still read and wrote the machine's real store. The IMAP bridge test wrote two
// messages under the account "sides", found four on its next run — its own two
// plus the two it had left behind a week earlier — and reported that the bridge
// duplicates every message. It does not. Three tests failed for a cause that
// was in neither the code they tested nor the code they were written in.
//
// The half of that which was worse than a red test: the flusher wrote back a
// second later, so `go test` deposited fixtures into a real person's
// conversations. This test fails if the record is ever bound at init again.
func TestTheRecordFollowsHome(t *testing.T) {
	first := t.TempDir()
	t.Setenv("HOME", first)

	th := Open("home_a", SMSClient, "+447700900001")
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	Add(Message{Thread: th.ID, Account: "home_a", Text: "in the first home"})
	if got := len(List("home_a", 10)); got != 1 {
		t.Fatalf("the first home has %d conversations, want 1", got)
	}

	// A different HOME is a different machine as far as this is concerned.
	second := t.TempDir()
	t.Setenv("HOME", second)
	if got := len(List("home_a", 10)); got != 0 {
		t.Errorf("a fresh home already has %d conversations in it — the record "+
			"is bound to whatever HOME was at process start, not to HOME", got)
	}

	// And what is written there stays there.
	th2 := Open("home_b", SMSClient, "+447700900002")
	if th2 == nil {
		t.Fatal("could not open a conversation in the second home")
	}
	Add(Message{Thread: th2.ID, Account: "home_b", Text: "in the second home"})
	Flush()

	if _, err := os.Stat(filepath.Join(second, ".mu", "data", "threads.json")); err != nil {
		t.Errorf("nothing was written to the home in effect: %v", err)
	}

	// Back to the first, and the first is still itself.
	t.Setenv("HOME", first)
	if got := len(List("home_b", 10)); got != 0 {
		t.Errorf("the second home's conversations followed us back to the first")
	}
}

// Flush writes to the home the record was read from, and to no other.
//
// The flusher ticks once a second on its own goroutine. t.Setenv restores HOME
// when a test ends, so a tick that falls due in the gap between two tests would
// find the real HOME and write the finished test's fixtures into it. That is
// not hypothetical: it is how thirty-seven fixture accounts reached a real
// store. Flush declining to write to a home it did not read from is what closes
// that window.
func TestFlushDoesNotFollowHomeSomewhereElse(t *testing.T) {
	mine := t.TempDir()
	t.Setenv("HOME", mine)

	th := Open("flush_scope", SMSClient, "+447700900003")
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	Add(Message{Thread: th.ID, Account: "flush_scope", Text: "stays here"})

	// Somebody else's home, as the gap between two tests would produce.
	elsewhere := t.TempDir()
	os.Setenv("HOME", elsewhere)
	Flush()
	os.Setenv("HOME", mine)

	if _, err := os.Stat(filepath.Join(elsewhere, ".mu", "data", "threads.json")); err == nil {
		t.Error("a flush wrote the record into a home it was never read from")
	}
}
