package wallet

import (
	"sync"

	"mu/internal/data"
)

// Pay-with-wallet preference: when on, an account's metered tool calls settle
// from its Base wallet (USDC) instead of drawing credits.

var (
	prefMu        sync.RWMutex
	payWithWallet = map[string]bool{} // accountID → on
	prefsFile     = "wallet_prefs.json"
	prefsOnce     sync.Once
)

func loadPrefs() {
	prefsOnce.Do(func() {
		prefMu.Lock()
		defer prefMu.Unlock()
		data.LoadJSON(prefsFile, &payWithWallet)
	})
}

// PayWithWallet reports whether the account pays for tools from its wallet.
func PayWithWallet(accountID string) bool {
	loadPrefs()
	prefMu.RLock()
	defer prefMu.RUnlock()
	return payWithWallet[accountID]
}

// SetPayWithWallet turns pay-with-wallet on or off for the account.
func SetPayWithWallet(accountID string, on bool) {
	loadPrefs()
	prefMu.Lock()
	defer prefMu.Unlock()
	if on {
		payWithWallet[accountID] = true
	} else {
		delete(payWithWallet, accountID)
	}
	data.SaveJSON(prefsFile, payWithWallet)
}
