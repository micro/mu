package stream

// Every test in this package runs against a scratch $HOME.
//
// add() calls save() on every entry, and save writes stream.json under the
// data directory — which resolves under $HOME on every call. Without this the
// suite overwrites whatever timeline the machine it runs on happens to have.
// The same omission in service/mail cost a real mailbox; see its main_test.go.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-stream-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
