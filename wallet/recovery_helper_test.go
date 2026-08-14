package wallet

import (
	"testing"
	"time"

	"mu/internal/data"
)

func writeStore(t *testing.T, name string, m map[string]*BaseWallet) {
	t.Helper()
	if err := data.SaveJSON(name, m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { data.DeleteFile(name) }) //nolint:errcheck
}

// walletTestState saves and restores the package globals these tests bend.
type walletTestState struct {
	wallets      map[string]*Wallet
	transactions map[string][]*Transaction
}

func (s *walletTestState) save(t *testing.T) {
	t.Helper()
	s.wallets, s.transactions = wallets, transactions
}

func (s *walletTestState) restore() { wallets, transactions = s.wallets, s.transactions }

func timeAt(n int) time.Time { return time.Date(2026, 8, 14, 10, n, 0, 0, time.UTC) }
