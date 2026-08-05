package app

import (
	"os"
	"testing"
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
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-app-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
