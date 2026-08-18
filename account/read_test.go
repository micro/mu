package account

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Reading a balance must not write one.
//
// Balance went through creditsOf, which creates a record when there is none —
// and creating one saves the whole of wallets.json. So a page that shows what
// every account holds wrote the file once per account it had never seen, each
// write serialised behind the ledger lock. That is /admin/users.
func TestAskingForABalanceDoesNotCreateOne(t *testing.T) {
	const who = "nobody-has-ever-topped-this-up"

	if n := Balance(who); n != 0 {
		t.Fatalf("an account with no record holds %d, want 0", n)
	}

	mutex.Lock()
	_, recorded := balances[who]
	mutex.Unlock()
	if recorded {
		t.Fatal("asking for a balance created a ledger row for it")
	}
}

// And the file is not touched by the asking.
func TestAskingForABalanceDoesNotWriteTheLedgerFile(t *testing.T) {
	path := filepath.Join(os.Getenv("HOME"), ".mu", "data", "wallets.json")

	// Establish a file to watch: one real write, then read a hundred unknown
	// accounts and see whether anything moved.
	if err := AddCredits("someone-who-paid", 10, "test_grant", nil); err != nil {
		t.Skipf("cannot exercise the ledger here: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Skipf("no ledger file to watch: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	for i := 0; i < 100; i++ {
		Balance("unknown-account-" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("ledger file gone: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("reading balances rewrote wallets.json")
	}
}
