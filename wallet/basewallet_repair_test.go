package wallet

// A wallet record with a key but no address.
//
// It came back from GetOrCreateWallet with no error and everything downstream
// rendered an empty string where an address goes — a blank button and a QR code
// of nothing, on a card that otherwise looked fine.

import "testing"

func TestBlankAddressIsRepairedFromTheKey(t *testing.T) {
	priv, addr, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	loadWallets()
	walletMu.Lock()
	userWallets["repairme"] = &BaseWallet{PrivateKey: priv} // address lost
	walletMu.Unlock()

	got, err := GetOrCreateWallet("repairme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != addr {
		t.Errorf("address %q, want it derived back to %q", got.Address, addr)
	}
	if got.PrivateKey != priv {
		t.Error("minted a new key instead of repairing — that would strand any USDC")
	}
}
