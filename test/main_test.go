package test

// A home of its own, because some of these tests move money.
//
// Most of this package reads source files and the service registry and touches
// no state at all. But TestFreeStaysFree tops an account up to prove a free
// operation is not charged, and `data` resolves every path under $HOME/.mu/data
// — so running the architecture tests wrote real credits and a real transaction
// into whatever ledger the machine had.
//
// Nothing here reads the data directory expecting to find anything, so pointing
// it at an empty one costs nothing and stops `go test ./test/` from being a
// thing that edits your balances.
//
// This only redirects writes. The account package's init has already loaded the
// real ledger into memory by the time TestMain runs — package initialisation
// happens first and there is no hook in front of it — and that is fine, because
// the point is that the files on disk are not touched.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-arch-test")
	if err != nil {
		panic("architecture tests need a scratch home: " + err.Error())
	}
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		panic("architecture tests need a scratch home: " + err.Error())
	}
	os.Setenv("HOME", home)

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
