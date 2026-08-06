package auth

import (
	"os"
	"testing"
)

// TestMain points HOME at a temporary directory so tests that create accounts
// and tokens do not write into the real ~/.mu.
//
// This fixes writing, not reading: auth's init() loads accounts.json and
// tokens.json when the package is linked, which happens before this runs. That
// is harmless — the tests here create what they need — but it is why a test
// must never assume the store starts empty.
//
// Written after a test in this session minted real tokens on a real account,
// which then had to be found and revoked by hand.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-auth-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
