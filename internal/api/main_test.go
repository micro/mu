package api

import (
	"os"
	"testing"
)

// TestMain points HOME at a temporary directory so tests that create accounts
// and tokens do not write into the real ~/.mu. Same reason as the one in
// internal/auth: a test in this session minted real tokens on a real account.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-api-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
