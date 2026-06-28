package auth

import (
	"strings"
	"testing"
	"time"
)

func TestCanPostAccountAgeAndVerificationRules(t *testing.T) {
	originalVerificationRequired := VerificationRequired
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "new-user")
		delete(accounts, "old-user")
		delete(accounts, "verified-user")
		delete(accounts, "approved-user")
		mutex.Unlock()
		VerificationRequired = originalVerificationRequired
	})

	now := time.Now()
	mutex.Lock()
	accounts["new-user"] = &Account{ID: "new-user", Created: now.Add(-23 * time.Hour)}
	accounts["old-user"] = &Account{ID: "old-user", Created: now.Add(-25 * time.Hour)}
	accounts["verified-user"] = &Account{ID: "verified-user", Created: now.Add(-25 * time.Hour), EmailVerified: true}
	accounts["approved-user"] = &Account{ID: "approved-user", Created: now, Approved: true}
	mutex.Unlock()

	VerificationRequired = func() bool { return false }
	if CanPost("new-user") {
		t.Fatal("expected new account to be blocked before 24 hour waiting period elapses")
	}
	if !CanPost("old-user") {
		t.Fatal("expected old account to post when verification is not required")
	}

	VerificationRequired = func() bool { return true }
	if CanPost("old-user") {
		t.Fatal("expected unverified account to be blocked when verification is required")
	}
	if !CanPost("verified-user") {
		t.Fatal("expected verified account to post when verification is required")
	}
	if !CanPost("approved-user") {
		t.Fatal("expected approved account to bypass age and verification restrictions")
	}
}

func TestPostBlockReasonExplainsAccountAgeBeforeVerification(t *testing.T) {
	originalVerificationRequired := VerificationRequired
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "blocked-user")
		mutex.Unlock()
		VerificationRequired = originalVerificationRequired
	})

	mutex.Lock()
	accounts["blocked-user"] = &Account{ID: "blocked-user", Created: time.Now().Add(-2 * time.Hour)}
	mutex.Unlock()
	VerificationRequired = func() bool { return true }

	reason := PostBlockReason("blocked-user")
	if !strings.Contains(reason, "New accounts must wait 24 hours") {
		t.Fatalf("expected age-based block reason, got %q", reason)
	}
	if strings.Contains(reason, "Verify your email") {
		t.Fatalf("expected age restriction to be reported before verification, got %q", reason)
	}
}
