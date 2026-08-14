package auth

import (
	"os"
	"testing"
)

// TestMain points HOME at a temporary directory so tests that create accounts
// and tokens do not write into the real ~/.mu.
//
// Written after a test in this session minted real tokens on a real account,
// which then had to be found and revoked by hand.
//
// Redirecting HOME fixed the writing and left the reading: auth's init() loads
// accounts.json and tokens.json when the package is linked, which happens
// before TestMain runs, so the store came up holding whatever accounts the
// machine had. That is not harmless. It made the suite depend on history —
// TestUsernameLookupIsCaseInsensitiveAndTrimmed failed with "Account already
// exists" on any box that had run it before, because the account it creates was
// still in the real file that init had read.
//
// So the maps are emptied too. There is no hook in front of package
// initialisation; what there is, is this.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-auth-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)

	mutex.Lock()
	accounts = map[string]*Account{}
	sessions = map[string]*Session{}
	tokens = map[string]*Token{}
	mutex.Unlock()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
