package app

import (
	"os"
	"testing"

	"mu/internal/auth"
)

// TestMain sends this package's writes to a temporary home.
//
// Accounts and tokens persist under $HOME/.mu and these tests create both, so
// without this they wrote into a real home directory and read their own
// leftovers back on the next run: a test that creates a token at the end passed
// the first time and failed the second, because the account it had just made
// already had one.
//
// It fixes the writing, not the reading. auth loads its store in an init(),
// which runs before this does, so whatever is on disk at start is already in
// memory. Nothing accumulates any more, which is what made the failure, but a
// test that needs a genuinely empty store cannot get one this way.
// It also claims the admin bootstrap. auth.Create promotes the first account on
// an instance with none to admin, and an admin is shown "unlimited" rather than
// a balance — so on a machine with an empty store, whichever test created the
// first account silently got one that behaves differently from the one it was
// written about. That is exactly how TestAnEmptyBalanceSaysHowToStart passed
// here and failed in CI for weeks: this machine had accounts on disk from
// earlier runs, and the CI runner did not.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-app-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	auth.Create(&auth.Account{ID: "bootstrap", Name: "bootstrap", Secret: "s"})
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
