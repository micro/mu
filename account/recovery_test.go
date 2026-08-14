package account

// Getting back what the collision destroyed.
//
// wallets.json was written by two different maps — this ledger and the key
// store in the wallet service, which used the same filename — so whichever
// saved last destroyed the other. Accounts came back with a balance of zero
// having done nothing to spend it. The other half of this story is in
// service/wallet/recovery_test.go.

import (
	"testing"
	"time"
)

// walletTestState saves and restores the package globals these tests bend.
type ledgerTestState struct {
	balances     map[string]*Credits
	transactions map[string][]*Transaction
}

func (s *ledgerTestState) save(t *testing.T) {
	t.Helper()
	s.balances, s.transactions = balances, transactions
}

func (s *ledgerTestState) restore() { balances, transactions = s.balances, s.transactions }

func timeAt(n int) time.Time { return time.Date(2026, 8, 14, 10, n, 0, 0, time.UTC) }

func TestBalancesAreRebuiltOverAPoisonedLedger(t *testing.T) {
	// What is on disk after the collision: an entry per account, decoded from
	// the key store's records, with no id and no balance. It is not empty, so
	// "does this account have a balance" answered yes and the rebuild skipped
	// everybody.
	mu := &ledgerTestState{}
	mu.save(t)
	defer mu.restore()

	balances = map[string]*Credits{
		"asim": {}, "someone": {}, // poisoned: decoded into nothing
	}
	for id, w := range balances {
		if w == nil || (w.UserID == "" && w.Balance == 0) {
			delete(balances, id)
		}
	}
	transactions = map[string][]*Transaction{
		"asim": {
			{UserID: "asim", Balance: 100, CreatedAt: timeAt(1)},
			{UserID: "asim", Balance: 560, CreatedAt: timeAt(3)},
			{UserID: "asim", Balance: 300, CreatedAt: timeAt(2)}, // out of order
		},
	}

	rebuildFromTransactions()

	if balances["asim"] == nil {
		t.Fatal("the balance was not rebuilt")
	}
	if got := balances["asim"].Balance; got != 560 {
		t.Errorf("rebuilt to %d, want 560 — the newest transaction's balance", got)
	}
}

func TestASurvivingBalanceIsNotRolledBack(t *testing.T) {
	// A top-up made after the last recorded transaction must win. An entry that
	// survived is authoritative.
	mu := &ledgerTestState{}
	mu.save(t)
	defer mu.restore()

	balances = map[string]*Credits{"asim": {UserID: "asim", Balance: 900}}
	transactions = map[string][]*Transaction{
		"asim": {{UserID: "asim", Balance: 560, CreatedAt: timeAt(1)}},
	}

	rebuildFromTransactions()

	if got := balances["asim"].Balance; got != 900 {
		t.Errorf("rolled a live balance back to %d, want 900", got)
	}
}
