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
