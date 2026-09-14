package auth

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentOwnedClientRegistrationIsBounded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "bounded-client-owner"
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := RegisterOwnedOAuthClient(owner, "Client", nil)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrCredentialLimit) {
				t.Errorf("registration: %v", err)
			}
		}()
	}
	wg.Wait()
	t.Cleanup(func() {
		for _, c := range OAuthClientsFor(owner) {
			DeleteOAuthClient(c.ClientID, owner)
		}
	})
	if n := accepted.Load(); n != 10 {
		t.Fatalf("accepted %d clients, want 10", n)
	}
	if _, err := RegisterOwnedOAuthClient("another-client-owner", "Client", nil); err != nil {
		t.Fatal(err)
	}
}

func TestTokenLimitCountsOnlyOwnersActiveTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	acc, _ := resetCSRFTestState(t)
	mutex.Lock()
	for i := range 20 {
		id := fmt.Sprintf("existing-%d", i)
		tokens[id] = &Token{ID: id, Account: acc.ID}
	}
	mutex.Unlock()
	if _, _, err := CreateToken(acc.ID, "Over limit", nil, time.Time{}); !errors.Is(err, ErrCredentialLimit) {
		t.Fatalf("limit: %v", err)
	}
	mutex.Lock()
	tokens["existing-0"].ExpiresAt = time.Now().Add(-time.Hour)
	mutex.Unlock()
	if _, _, err := CreateToken(acc.ID, "Replacement", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
}
