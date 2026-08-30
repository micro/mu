package arrivals

// A home of its own, because counting the archive opens it.
//
// The first read initialises the index under $HOME/.mu/data, which on a
// developer box is their real one — and these tests then put rows in it. Pin
// the directory down before anything asks a question of it.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-arrivals-test")
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
