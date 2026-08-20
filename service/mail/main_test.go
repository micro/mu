package mail

// Every test in this package runs against a scratch $HOME.
//
// It did not, and the cost was mail. `markSelfSentRead` calls save() when it
// changes anything, and TestMarkingSelfSentReadLeavesRealMailAlone hands it
// three fixtures with a self-sent one among them — so running the suite wrote
// {"id":"1"},{"id":"2"},{"id":"3"} over ~/.mu/data/mail.json and every real
// message in it was gone. Found by delivering three messages to an instance,
// reading them back over IMAP, running `go test ./...`, and restarting: three
// messages, none of them decryptable, none of them mine.
//
// Accounts too. outbound_test.go calls auth.Create, which writes accounts.json,
// so a test account landed in whatever account store the machine had.
//
// test/test_home_test.go already enforces this for any package that can reach
// the ledger. A service may not import account/, so mail was outside a rule
// whose own comment says it cannot catch everything. Balances can be restated
// from the transactions. Mail cannot be restated from anything.
//
// Package-wide rather than per-test, because the next test to write here will
// not remember: data resolves paths under $HOME on every call, so moving HOME
// once moves the whole store for the whole binary.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-mail-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
