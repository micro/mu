package account

// Prices come from above: main embeds quota.json and hands it to internal/quota.
// A test binary has no main, so the cost tables here would render from the
// 1-credit fallback and every assertion about what something costs would be
// agreeing with itself about nothing.
//
// And a home of its own, because the tests in this package move money. They
// credit accounts, transfer between them and settle payments, and data resolves
// every path under $HOME/.mu/data — so `go test ./account/` was writing real
// balances and real transactions into whatever ledger the machine had. It also
// meant the tests were not repeatable: a top-up left behind by the last run was
// still there for the next one, which is how a durable settlement check made
// two of them start failing rather than the check being wrong.

import (
	"os"
	"path/filepath"
	"testing"

	"mu/internal/quota"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-account-test")
	if err != nil {
		panic("account tests need a scratch home: " + err.Error())
	}
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		panic("account tests need a scratch home: " + err.Error())
	}
	os.Setenv("HOME", home)

	// init() has already run, against the real home, so drop what it loaded.
	// Package initialisation happens before TestMain and there is no hook to
	// get in front of it; what there is, is this — start from nothing, in a
	// directory nobody else is using.
	mutex.Lock()
	balances = map[string]*Credits{}
	transactions = map[string][]*Transaction{}
	dailyUsage = map[string]*DailyUsage{}
	mutex.Unlock()

	if err := quota.LoadFromTree(); err != nil {
		os.RemoveAll(home)
		panic("account tests need quota.json: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
