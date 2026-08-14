package account

// The ledger has one door.
//
// It was an RWMutex with every function locking by hand, and that shape is what
// produced the bug it eventually produced: CreditsOf took the read lock, let it
// go, took the write lock — and a credit landing in that gap was overwritten.
// Somebody opening /account while their top-up settled lost the top-up.
//
// Nothing was wrong with any individual line. Reviewing more carefully was
// never going to be the fix, because the design invited the mistake: nine
// functions each deciding for themselves when to hold a lock, and one read lock
// available to upgrade from.
//
// So there is one Mutex, one function that takes it, and a *ledger token that
// cannot be made anywhere else. A helper that touches balances or transactions
// takes that token, which means the compiler refuses to let it be called
// without the lock held — where before there was a comment saying "callers hold
// mutex", which is the same claim with nobody checking it.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var locks = regexp.MustCompile(`mutex\.(RLock|Lock|RUnlock|Unlock)\(\)`)

func TestTheLedgerHasOneDoor(t *testing.T) {
	src, err := os.ReadFile("credits.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// The only permitted lock site is withLedger itself.
	i := strings.Index(body, "func withLedger[")
	if i < 0 {
		t.Fatal("withLedger is gone — the ledger has no door, so every caller is " +
			"deciding for itself when to hold a lock again")
	}
	end := strings.Index(body[i:], "\n}\n")
	if end < 0 {
		t.Fatal("cannot find the end of withLedger")
	}
	door := body[i : i+end]

	outside := body[:i] + body[i+end:]
	if m := locks.FindAllString(outside, -1); len(m) > 0 {
		t.Errorf("credits.go locks the mutex outside withLedger (%d times: %v) — that "+
			"is how a read, an unlock and a write end up on either side of somebody "+
			"else's credit", len(m), m)
	}
	if !locks.MatchString(door) {
		t.Error("withLedger does not take the lock")
	}
}

// A plain Mutex, because there is no read lock left to upgrade from — and an
// upgrade is exactly what the lost update was.
func TestTheLedgerLockIsExclusive(t *testing.T) {
	src, err := os.ReadFile("credits.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "var mutex sync.Mutex") {
		t.Error("the ledger lock is not a plain sync.Mutex — an RWMutex offers a read " +
			"lock, and taking one then reaching for the write lock is the bug this " +
			"package already shipped")
	}
}

// Everything that touches the maps takes the token, so the compiler is what
// enforces this rather than a comment.
func TestNothingClaimsTheLockInAComment(t *testing.T) {
	src, err := os.ReadFile("credits.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"Callers hold mutex", "callers hold the lock", "lock already held"} {
		if strings.Contains(string(src), phrase) {
			t.Errorf("a comment says %q. Take a *ledger instead — the point of the "+
				"token is that the claim stops being a promise", phrase)
		}
	}
}
