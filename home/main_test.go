package home

// A home of its own, because reading a balance writes one.
//
// These tests render the usage page, which asks the account what it has. That
// looks like a read and is not: CreditsOf creates a record when there is none
// and saves it, so simply asking about an account that has never paid for
// anything writes the ledger — under $HOME/.mu/data, which on a developer box
// is their real one.
//
// The pages are the point of this package and they will keep reaching for a
// balance, so the directory they reach into is the thing to pin down.

import (
	"os"
	"path/filepath"
	"testing"

	"mu/internal/quota"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-home-test")
	if err != nil {
		panic("tests need a scratch home: " + err.Error())
	}
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		panic("tests need a scratch home: " + err.Error())
	}
	os.Setenv("HOME", home)

	// And the price list, which reaches this package from above: main embeds
	// quota.json and calls Load, and a test binary has no main. Without it
	// every operation costs the 1-credit fallback and /pricing renders a table
	// of nothing — which is a page that would pass a test asserting it exists
	// and show a stranger no prices at all.
	if err := quota.LoadFromTree(); err != nil {
		panic("tests need quota.json: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
