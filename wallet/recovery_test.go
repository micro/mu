package wallet

// Getting back what the collision destroyed.

import "testing"

func TestAFileOfEmptyRecordsIsNotAWalletSet(t *testing.T) {
	// Exactly what the credit ledger decoded into: an entry per account, every
	// field empty, because the field names belong to a different struct. The
	// map looked populated, so the fallback to the file that had the real keys
	// was never reached.
	poisoned := map[string]*BaseWallet{
		"alice": {}, "bob": {}, "carol": nil,
	}
	if got := usable(poisoned); len(got) != 0 {
		t.Errorf("kept %d empty records, so a real key store would never be reached", len(got))
	}

	real := map[string]*BaseWallet{
		"alice": {Address: "0xabc", PrivateKey: "ff"},
		"bob":   {},
	}
	got := usable(real)
	if len(got) != 1 || got["alice"] == nil {
		t.Errorf("dropped a usable wallet: %+v", got)
	}
}

func TestAPoisonedPrimaryFallsBackToTheLegacyFile(t *testing.T) {
	// The recovery this instance needed: wallets.json full of empty records,
	// trade_wallets.json holding the keys to real money.
	writeStore(t, "poisoned_primary.json", map[string]*BaseWallet{"asim": {}})
	writeStore(t, "poisoned_legacy.json", map[string]*BaseWallet{
		"asim": {Address: "0x0537d281", PrivateKey: "deadbeef"},
	})

	m := loadWalletsFrom("poisoned_primary.json", "poisoned_legacy.json")
	if len(m) != 1 || m["asim"] == nil || m["asim"].Address != "0x0537d281" {
		t.Fatalf("did not recover the real keys: %+v", m)
	}
	// And it copies forward, so the legacy file stops being load-bearing.
	forward := loadWalletsFrom("poisoned_primary.json", "does_not_exist.json")
	if forward["asim"] == nil || forward["asim"].Address != "0x0537d281" {
		t.Error("the recovered keys were not written to the primary file")
	}
}

func TestBalancesAreRebuiltOverAPoisonedLedger(t *testing.T) {
	// What is on disk after the collision: an entry per account, decoded from
	// the key store's records, with no id and no balance. It is not empty, so
	// "does this account have a wallet" answered yes and the rebuild skipped
	// everybody.
	mu := &walletTestState{}
	mu.save(t)
	defer mu.restore()

	wallets = map[string]*Wallet{
		"asim": {}, "someone": {}, // poisoned: decoded into nothing
	}
	for id, w := range wallets {
		if w == nil || (w.UserID == "" && w.Balance == 0) {
			delete(wallets, id)
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

	if wallets["asim"] == nil {
		t.Fatal("the balance was not rebuilt")
	}
	if got := wallets["asim"].Balance; got != 560 {
		t.Errorf("rebuilt to %d, want 560 — the newest transaction's balance", got)
	}
}

func TestASurvivingBalanceIsNotRolledBack(t *testing.T) {
	// A top-up made after the last recorded transaction must win. An entry that
	// survived is authoritative.
	mu := &walletTestState{}
	mu.save(t)
	defer mu.restore()

	wallets = map[string]*Wallet{"asim": {UserID: "asim", Balance: 900}}
	transactions = map[string][]*Transaction{
		"asim": {{UserID: "asim", Balance: 560, CreatedAt: timeAt(1)}},
	}

	rebuildFromTransactions()

	if got := wallets["asim"].Balance; got != 900 {
		t.Errorf("rolled a live balance back to %d, want 900", got)
	}
}
