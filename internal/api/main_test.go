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

	// Snapshot the real tool surface before any test body runs.
	//
	// Several tests register probe services to exercise derivation, and those
	// probes are deliberately terse. Linting the live registry would therefore
	// pass or fail depending on which tests had run first — the same
	// order-dependence that made two other tests in this repo lie today. The
	// lint reads this instead, which is the surface an agent actually sees.
	shipped = mcpTools()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
