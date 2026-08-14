package wallet

// Never write a key store that has lost keys.
//
// The keys to real money were destroyed once, and the mechanism was ordinary: a
// map that had decoded into nothing was handed to SaveJSON, and SaveJSON did
// exactly what it was told. Atomic writes did not help — the write was
// perfectly atomic and perfectly wrong. Nothing between the mistake and the
// disk asked whether losing every key was intended.
//
// This asks. A save that would drop a key is refused unless the caller says it
// is deleting one, and it says so by calling the deleting function. That turns
// the worst failure this code can have into a log line and a returned error,
// with the previous file still on disk.
//
// It is not a general rule for every store, and should not be: plenty of files
// here legitimately shrink. It is a rule for the one file whose contents cannot
// be reconstructed from anything else. A balance can be rebuilt from the
// transaction log. A private key cannot be rebuilt from anything — that is what
// makes it a private key.

import (
	"fmt"

	"mu/internal/app"
	"mu/internal/data"
)

// saveWallets persists the key store, refusing a write that would lose keys.
//
// Callers hold walletMu.
func saveWallets() error {
	return saveWalletsAllowing(0)
}

// saveWalletsAllowing persists the key store, permitting up to drop keys to
// disappear. Deleting an account's wallet passes 1.
//
// Callers hold walletMu.
func saveWalletsAllowing(drop int) error {
	// What is on disk right now, read back rather than remembered. A count held
	// in memory is a second copy of the truth and would have been wrong in
	// exactly the situation this exists for.
	onDisk := len(usable(loadRaw(walletsFile)))
	have := len(usable(userWallets))

	if have+drop < onDisk {
		err := fmt.Errorf("refusing to write the key store: it holds %d usable wallets "+
			"and the file holds %d, so this save would lose %d private keys",
			have, onDisk, onDisk-have-drop)
		// Loudly. A refusal nobody sees is a corruption nobody sees, one deploy
		// later, when whatever caused this is still there.
		app.Log("wallet", "CRITICAL: %v", err)
		return err
	}

	if err := data.SaveJSON(walletsFile, userWallets); err != nil {
		app.Log("wallet", "CRITICAL: could not write the key store: %v", err)
		return err
	}
	return nil
}
