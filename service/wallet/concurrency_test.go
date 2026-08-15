package wallet

// Two requests at once must not mint two wallets for one account.
//
// The way this goes wrong is not exotic. An agent calls wallet_address while
// the page renders wallet_balance; both find no wallet; both generate a keypair;
// the second one stored wins and the first key is gone. If anything was sent to
// the first address in between — and the first caller was told that address —
// the funds are at a key nobody has.
//
// Run with -race.

import (
	"sync"
	"testing"
)

func TestConcurrentFirstUseMintsOneWallet(t *testing.T) {
	const id = "wallet-race-new"
	const callers = 25

	got := make([]string, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bw, err := GetOrCreateWallet(id)
			if err != nil || bw == nil {
				return
			}
			got[i] = bw.Address
		}(i)
	}
	wg.Wait()

	first := ""
	for i, addr := range got {
		if addr == "" {
			t.Fatalf("caller %d got no address", i)
		}
		if first == "" {
			first = addr
		}
		if addr != first {
			t.Fatalf("caller %d was told %s, caller 0 was told %s — two wallets were "+
				"minted for one account, so whichever was told the losing address was "+
				"told to send money to a key that is no longer stored", i, addr, first)
		}
	}

	// And the address handed out is the one actually kept.
	if kept := For(id); kept == nil || kept.Address != first {
		t.Errorf("callers were told %s, the store holds %v", first, kept)
	}
}

// Reading a wallet while its address is repaired must not race.
//
// GetOrCreateWallet writes an address back onto a record that lost one. A caller
// holding the stored record rather than a copy reads the key while that write
// happens.
func TestReadingAWalletWhileItIsRepaired(t *testing.T) {
	const id = "wallet-race-repair"
	priv, _, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	loadWallets()
	walletMu.Lock()
	userWallets[id] = &BaseWallet{PrivateKey: priv} // address lost
	walletMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := For(id); w != nil {
				_ = w.PrivateKey
				_ = w.Address
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetOrCreateWallet(id) //nolint:errcheck
		}()
	}
	wg.Wait()

	// The key must be the one we put there. A repair that minted instead would
	// strand whatever the old key held.
	if w := For(id); w == nil || w.PrivateKey != priv {
		t.Error("the key was replaced rather than repaired")
	}
}
