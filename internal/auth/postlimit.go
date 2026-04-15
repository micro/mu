// Per-account post rate limiting and email verification token storage.
// Sits in the auth package so handlers in social/blog/apps can call into
// a single place without import cycles.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"mu/internal/data"
)

// ============================================================
// Per-account post rate limiter
// ============================================================

// postLimitBucket tracks how many content actions an account has
// performed inside the current window.
type postLimitBucket struct {
	count   int
	resetAt time.Time
}

var (
	postLimitMu sync.Mutex
	postBuckets = map[string]*postLimitBucket{}
)

// postLimitFor returns (max, window) for the given account based on age.
// During the first 24 hours after signup an account is on a tight cap;
// after that it gets a generous hourly cap that still throttles abuse.
// Admins and approved accounts are unlimited.
func postLimitFor(acc *Account) (int, time.Duration) {
	if acc.Admin || acc.Approved {
		return 1<<31 - 1, time.Hour
	}
	if time.Since(acc.Created) < 24*time.Hour {
		// New account: 10 actions per hour by default. Also caps the
		// theoretical 24h ceiling at 240 — combined with the per-action
		// credit cost, this is enough to stop a runaway bot.
		return envIntAuth("NEW_POST_LIMIT_PER_HOUR", 10), time.Hour
	}
	return envIntAuth("POST_LIMIT_PER_HOUR", 60), time.Hour
}

// CheckPostRate returns nil if the account is allowed to create another
// piece of content right now, otherwise an error explaining the limit.
// This is a sliding-bucket limiter: the count resets after the window
// elapses from the first action in the bucket.
func CheckPostRate(accountID string) error {
	mutex.Lock()
	acc, exists := accounts[accountID]
	mutex.Unlock()
	if !exists {
		return errors.New("account not found")
	}

	max, window := postLimitFor(acc)

	postLimitMu.Lock()
	defer postLimitMu.Unlock()

	now := time.Now()
	b, ok := postBuckets[accountID]
	if !ok || now.After(b.resetAt) {
		b = &postLimitBucket{count: 0, resetAt: now.Add(window)}
		postBuckets[accountID] = b
	}
	if b.count >= max {
		remaining := time.Until(b.resetAt).Round(time.Minute)
		if remaining < time.Minute {
			remaining = time.Minute
		}
		return fmt.Errorf("post rate limit reached (%d per %s). Try again in %s", max, window, remaining)
	}
	b.count++

	// Opportunistic GC.
	if len(postBuckets) > 50000 {
		for k, v := range postBuckets {
			if now.After(v.resetAt) {
				delete(postBuckets, k)
			}
		}
	}
	return nil
}

// ============================================================
// Email verification tokens
// ============================================================

type emailToken struct {
	AccountID string
	Email     string
	ExpiresAt time.Time
}

var (
	emailTokenMu sync.Mutex
	emailTokens  = map[string]*emailToken{}
)

// CreateEmailVerificationToken issues a one-time verification token for
// the given account/email pair. Any prior token for the same account is
// invalidated so re-requesting cancels the old one.
func CreateEmailVerificationToken(accountID, email string) (string, error) {
	mutex.Lock()
	_, exists := accounts[accountID]
	mutex.Unlock()
	if !exists {
		return "", errors.New("account not found")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(tokenBytes)

	emailTokenMu.Lock()
	// Invalidate any previous tokens for this account.
	for k, v := range emailTokens {
		if v.AccountID == accountID {
			delete(emailTokens, k)
		}
	}
	emailTokens[tok] = &emailToken{
		AccountID: accountID,
		Email:     email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	emailTokenMu.Unlock()
	return tok, nil
}

// ConsumeEmailVerificationToken verifies a token and, on success, marks
// the account's email as verified. The token is single-use.
func ConsumeEmailVerificationToken(token string) (*Account, error) {
	emailTokenMu.Lock()
	t, ok := emailTokens[token]
	if ok {
		delete(emailTokens, token)
	}
	emailTokenMu.Unlock()
	if !ok {
		return nil, errors.New("invalid or expired verification link")
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, errors.New("verification link has expired — please request a new one")
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, exists := accounts[t.AccountID]
	if !exists {
		return nil, errors.New("account no longer exists")
	}
	acc.Email = t.Email
	acc.EmailVerified = true
	acc.EmailVerifiedAt = time.Now()
	if err := saveAccountsUnlocked(); err != nil {
		return nil, err
	}
	return acc, nil
}

// SetAccountEmail stores an unverified email on the account (called when
// the user submits the verification form so we remember the pending
// address even before they click the link).
func SetAccountEmail(accountID, email string) error {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return errors.New("account not found")
	}
	acc.Email = email
	acc.EmailVerified = false
	return saveAccountsUnlocked()
}

// saveAccountsUnlocked persists the accounts map. Caller must hold mutex.
func saveAccountsUnlocked() error {
	return data.SaveJSON("accounts.json", accounts)
}

// envIntAuth is a tiny helper so this file doesn't depend on app.envInt.
func envIntAuth(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}
