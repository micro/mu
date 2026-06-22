package discord

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type linkCode struct {
	Account   string
	ExpiresAt time.Time
}

var (
	codeMu sync.Mutex
	codes  = map[string]*linkCode{} // code → account
)

// GenerateLinkCode creates a one-time code that a user pastes in Discord
// to link their account. Expires after 5 minutes.
func GenerateLinkCode(accountID string) string {
	codeMu.Lock()
	defer codeMu.Unlock()

	// Clear any existing codes for this account
	for k, v := range codes {
		if v.Account == accountID || time.Now().After(v.ExpiresAt) {
			delete(codes, k)
		}
	}

	b := make([]byte, 4)
	rand.Read(b)
	code := hex.EncodeToString(b)

	codes[code] = &linkCode{
		Account:   accountID,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	return code
}

// redeemCode validates and consumes a link code, returning the account ID.
func redeemCode(code string) (string, bool) {
	codeMu.Lock()
	defer codeMu.Unlock()

	lc, ok := codes[code]
	if !ok || time.Now().After(lc.ExpiresAt) {
		delete(codes, code)
		return "", false
	}

	account := lc.Account
	delete(codes, code)
	return account, true
}
