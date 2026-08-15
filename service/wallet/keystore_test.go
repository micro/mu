package wallet

// The disaster, replayed.
//
// A map that had decoded into nothing was handed to SaveJSON, and SaveJSON did
// what it was told: it atomically replaced a file full of private keys with a
// file full of empty records. The write was correct in every mechanical sense.
// Nothing asked whether losing every key was intended.

import (
	"testing"

	"mu/internal/data"
)

// restore puts the package globals back, so one test cannot poison the next.
func restore(t *testing.T) {
	t.Helper()
	walletMu.Lock()
	prev := userWallets
	walletMu.Unlock()
	t.Cleanup(func() {
		walletMu.Lock()
		userWallets = prev
		walletMu.Unlock()
	})
}

func TestAWriteThatWouldLoseKeysIsRefused(t *testing.T) {
	dataHome(t)
	restore(t)

	real := map[string]*BaseWallet{
		"alice": {Address: "0xaaa", PrivateKey: "11"},
		"bob":   {Address: "0xbbb", PrivateKey: "22"},
		"carol": {Address: "0xccc", PrivateKey: "33"},
	}
	if err := data.SaveJSON(walletsFile, real); err != nil {
		t.Fatal(err)
	}

	// Exactly what happened: the in-memory map decoded into nothing, because
	// another struct's field names were read into this one.
	walletMu.Lock()
	userWallets = map[string]*BaseWallet{"alice": {}, "bob": {}, "carol": {}}
	err := saveWallets()
	walletMu.Unlock()

	if err == nil {
		t.Fatal("a save that loses every key was allowed")
	}

	// And the file still has the keys.
	back := map[string]*BaseWallet{}
	if err := data.LoadJSON(walletsFile, &back); err != nil {
		t.Fatal(err)
	}
	if len(usable(back)) != 3 {
		t.Errorf("the file holds %d usable wallets, want 3 — the refusal did not "+
			"protect it", len(usable(back)))
	}
	if back["alice"] == nil || back["alice"].PrivateKey != "11" {
		t.Error("alice's key is gone")
	}
}

// Growing is fine. Nothing here should make adding a wallet harder.
func TestAddingAWalletSavesNormally(t *testing.T) {
	dataHome(t)
	restore(t)

	walletMu.Lock()
	userWallets = map[string]*BaseWallet{"alice": {Address: "0xaaa", PrivateKey: "11"}}
	err := saveWallets()
	walletMu.Unlock()
	if err != nil {
		t.Fatalf("a first write was refused: %v", err)
	}

	walletMu.Lock()
	userWallets["bob"] = &BaseWallet{Address: "0xbbb", PrivateKey: "22"}
	err = saveWallets()
	walletMu.Unlock()
	if err != nil {
		t.Fatalf("adding a wallet was refused: %v", err)
	}

	back := map[string]*BaseWallet{}
	if err := data.LoadJSON(walletsFile, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Errorf("the file holds %d wallets, want 2", len(back))
	}
}

// Deleting one is allowed, because that is what the caller asked for. Deleting
// one and losing another is not.
func TestDeletingOneWalletIsAllowedAndTwoIsNot(t *testing.T) {
	dataHome(t)
	restore(t)

	start := map[string]*BaseWallet{
		"alice": {Address: "0xaaa", PrivateKey: "11"},
		"bob":   {Address: "0xbbb", PrivateKey: "22"},
		"carol": {Address: "0xccc", PrivateKey: "33"},
	}
	if err := data.SaveJSON(walletsFile, start); err != nil {
		t.Fatal(err)
	}

	walletMu.Lock()
	userWallets = map[string]*BaseWallet{
		"alice": {Address: "0xaaa", PrivateKey: "11"},
		"bob":   {Address: "0xbbb", PrivateKey: "22"},
	}
	err := saveWalletsAllowing(1)
	walletMu.Unlock()
	if err != nil {
		t.Fatalf("deleting one wallet was refused: %v", err)
	}

	// Now two vanish at once while the caller claims to be deleting one.
	if err := data.SaveJSON(walletsFile, start); err != nil {
		t.Fatal(err)
	}
	walletMu.Lock()
	userWallets = map[string]*BaseWallet{"alice": {Address: "0xaaa", PrivateKey: "11"}}
	err = saveWalletsAllowing(1)
	walletMu.Unlock()
	if err == nil {
		t.Error("a save losing two keys was allowed under a one-key deletion")
	}
}

// DeleteBaseWallet still works, end to end.
func TestDeleteBaseWalletStillDeletes(t *testing.T) {
	dataHome(t)
	restore(t)

	walletMu.Lock()
	userWallets = map[string]*BaseWallet{
		"alice": {Address: "0xaaa", PrivateKey: "11"},
		"bob":   {Address: "0xbbb", PrivateKey: "22"},
	}
	walletsInit.Do(func() {}) // loadWallets has already run for this process
	err := saveWallets()
	walletMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	DeleteBaseWallet("bob")

	if For("bob") != nil {
		t.Error("the wallet was not deleted")
	}
	back := map[string]*BaseWallet{}
	if err := data.LoadJSON(walletsFile, &back); err != nil {
		t.Fatal(err)
	}
	if _, still := back["bob"]; still {
		t.Error("the deletion was not written")
	}
	if back["alice"] == nil {
		t.Error("deleting bob took alice with it")
	}
}
