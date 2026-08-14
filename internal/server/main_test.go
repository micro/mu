package server

// A home of its own, because these tests move money.
//
// data resolves every path under $HOME/.mu/data, so a test that credits an
// account writes into whatever ledger the machine has. That is somebody's real
// balance on a developer box, and it makes the tests unrepeatable besides — a
// grant left behind by the last run is still there for the next one.
//
// This redirects the writes. The account package's init has already read the
// real ledger into memory by the time TestMain runs, and that is fine: the point
// is that nothing on disk is touched.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-server-test")
	if err != nil {
		panic("tests need a scratch home: " + err.Error())
	}
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		panic("tests need a scratch home: " + err.Error())
	}
	os.Setenv("HOME", home)

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
