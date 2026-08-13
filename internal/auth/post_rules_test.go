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

// A blocked new account is told how to unblock itself, not just how long to
// wait. The wait used to be the whole message, which reads as a dead end when
// two doors are open right now.
func TestPostBlockReasonOffersTheWayOut(t *testing.T) {
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
	// Both ways out, named as places rather than as routes. A path written into
	// a sentence reads as a link and is not one; the renderer turns these words
	// into the links, so the sentence stays a sentence wherever it is shown.
	for _, want := range []string{"your Account", "your Wallet", "24 hours"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("expected the reason to mention %q, got %q", want, reason)
		}
	}
	if strings.Contains(reason, "/account") || strings.Contains(reason, "/wallet") {
		t.Errorf("the reason names a route, which reads as a link and is not one: %q", reason)
	}
}

// The 24-hour wait is a fallback for an account we know nothing about. A
// verified address or a funded wallet is the signal it was standing in for, so
// either one lets a brand-new account post immediately.
func TestTrustSignalsSkipTheWait(t *testing.T) {
	originalVerificationRequired := VerificationRequired
	originalHasCredit := HasCredit
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "fresh-verified")
		delete(accounts, "fresh-funded")
		delete(accounts, "fresh-broke")
		mutex.Unlock()
		VerificationRequired = originalVerificationRequired
		HasCredit = originalHasCredit
	})

	now := time.Now()
	mutex.Lock()
	accounts["fresh-verified"] = &Account{ID: "fresh-verified", Created: now, EmailVerified: true}
	accounts["fresh-funded"] = &Account{ID: "fresh-funded", Created: now}
	accounts["fresh-broke"] = &Account{ID: "fresh-broke", Created: now}
	mutex.Unlock()

	VerificationRequired = func() bool { return true }
	HasCredit = func(id string) bool { return id == "fresh-funded" }

	if !CanPost("fresh-verified") {
		t.Fatal("a verified address should clear the new-account wait")
	}
	if !CanPost("fresh-funded") {
		t.Fatal("credit in the wallet should clear the new-account wait")
	}
	if CanPost("fresh-broke") {
		t.Fatal("an account with neither signal should still wait")
	}
	if PostBlockReason("fresh-verified") != "" {
		t.Fatal("a trusted account should have no block reason")
	}

	// Whatever accepts a post must also show it. The blog hides posts by a
	// "new" account from its lists, so if IsNewAccount and CanPost disagree the
	// write succeeds, is charged, and then silently vanishes from the page the
	// author is redirected to.
	for _, id := range []string{"fresh-verified", "fresh-funded", "fresh-broke"} {
		if IsNewAccount(id) == CanPost(id) {
			t.Fatalf("%s: IsNewAccount and CanPost disagree — a post would be accepted and then hidden", id)
		}
	}

	// And the tight new-account rate cap follows the same signal: a trusted
	// account is capped at the established rate, not the bot-defence rate.
	mutex.Lock()
	trustedAcc := *accounts["fresh-funded"]
	untrusted := *accounts["fresh-broke"]
	mutex.Unlock()
	if trustedMax, _ := postLimitFor(trustedAcc); trustedMax <= 10 {
		t.Fatalf("expected a trusted new account off the new-account cap, got %d", trustedMax)
	}
	if untrustedMax, _ := postLimitFor(untrusted); untrustedMax > 10 {
		t.Fatalf("expected an untrusted new account on the tight cap, got %d", untrustedMax)
	}
}

// HasCredit reaches into the wallet, which reads accounts. Calling it under
// auth's own mutex would deadlock the first time anyone posted, so the trust
// rules snapshot the account and evaluate after releasing the lock. This test
// fails by hanging if that ever regresses.
func TestTrustRulesDoNotHoldTheLock(t *testing.T) {
	originalHasCredit := HasCredit
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "reentrant")
		mutex.Unlock()
		HasCredit = originalHasCredit
	})

	mutex.Lock()
	accounts["reentrant"] = &Account{ID: "reentrant", Created: time.Now()}
	mutex.Unlock()

	HasCredit = func(id string) bool {
		// What the wallet does: look the account up again.
		mutex.Lock()
		_, ok := accounts[id]
		mutex.Unlock()
		return ok
	}

	done := make(chan bool, 1)
	go func() { done <- CanPost("reentrant") }()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected the funded account to be allowed to post")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CanPost deadlocked: the trust rules are calling out under auth's mutex")
	}
}
