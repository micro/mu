package wallet

// Tests here write key stores, so they get a home of their own.
//
// data resolves every path under $HOME/.mu/data, read fresh on each call. This
// package used to hold the credit ledger too, so a test writing wallets.json
// was writing its own file; the ledger has moved to account/ and that same
// write would now land on somebody's real balances. It has happened once, with
// real money, and the fix costs three lines.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-wallet-test")
	if err != nil {
		panic("wallet tests need a scratch home: " + err.Error())
	}
	defer os.RemoveAll(home)
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		panic("wallet tests need a scratch home: " + err.Error())
	}
	os.Setenv("HOME", home)

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
