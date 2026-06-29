package auth

import (
	"strings"
	"testing"
	"time"
)

func resetPostLimitStateForTest(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NEW_POST_LIMIT_PER_HOUR", "")
	t.Setenv("POST_LIMIT_PER_HOUR", "")

	mutex.Lock()
	accounts = map[string]*Account{}
	mutex.Unlock()

	postLimitMu.Lock()
	postBuckets = map[string]*postLimitBucket{}
	postLimitMu.Unlock()

	emailTokenMu.Lock()
	emailTokens = map[string]*emailToken{}
	emailTokenMu.Unlock()
}

func TestCheckPostRateHonorsNewAccountLimit(t *testing.T) {
	resetPostLimitStateForTest(t)
	t.Setenv("NEW_POST_LIMIT_PER_HOUR", "2")

	mutex.Lock()
	accounts["new-account"] = &Account{ID: "new-account", Created: time.Now()}
	mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := CheckPostRate("new-account"); err != nil {
			t.Fatalf("CheckPostRate attempt %d returned error: %v", i+1, err)
		}
	}
	if err := CheckPostRate("new-account"); err == nil || !strings.Contains(err.Error(), "post rate limit reached (2 per 1h0m0s)") {
		t.Fatalf("CheckPostRate after limit = %v, want rate limit error", err)
	}
}

func TestCheckPostRateApprovedAccountBypassesLimit(t *testing.T) {
	resetPostLimitStateForTest(t)
	t.Setenv("NEW_POST_LIMIT_PER_HOUR", "1")

	mutex.Lock()
	accounts["approved-account"] = &Account{ID: "approved-account", Created: time.Now(), Approved: true}
	mutex.Unlock()

	for i := 0; i < 3; i++ {
		if err := CheckPostRate("approved-account"); err != nil {
			t.Fatalf("CheckPostRate approved attempt %d returned error: %v", i+1, err)
		}
	}
}

func TestEmailVerificationTokenInvalidatesPreviousAndIsSingleUse(t *testing.T) {
	resetPostLimitStateForTest(t)

	mutex.Lock()
	accounts["acct-1"] = &Account{ID: "acct-1", Created: time.Now()}
	mutex.Unlock()

	first, err := CreateEmailVerificationToken("acct-1", "old@example.com")
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken first returned error: %v", err)
	}
	second, err := CreateEmailVerificationToken("acct-1", "new@example.com")
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken second returned error: %v", err)
	}
	if first == second {
		t.Fatal("CreateEmailVerificationToken returned duplicate tokens")
	}

	if _, err := ConsumeEmailVerificationToken(first); err == nil {
		t.Fatal("ConsumeEmailVerificationToken accepted invalidated prior token")
	}

	acc, err := ConsumeEmailVerificationToken(second)
	if err != nil {
		t.Fatalf("ConsumeEmailVerificationToken returned error: %v", err)
	}
	if acc.Email != "new@example.com" || !acc.EmailVerified || acc.EmailVerifiedAt.IsZero() {
		t.Fatalf("ConsumeEmailVerificationToken did not mark account verified: %#v", acc)
	}
	if _, err := ConsumeEmailVerificationToken(second); err == nil {
		t.Fatal("ConsumeEmailVerificationToken accepted a reused token")
	}
}

func TestConsumeEmailVerificationTokenRejectsExpiredToken(t *testing.T) {
	resetPostLimitStateForTest(t)

	mutex.Lock()
	accounts["acct-1"] = &Account{ID: "acct-1", Created: time.Now()}
	mutex.Unlock()

	emailTokenMu.Lock()
	emailTokens["expired"] = &emailToken{AccountID: "acct-1", Email: "new@example.com", ExpiresAt: time.Now().Add(-time.Minute)}
	emailTokenMu.Unlock()

	if _, err := ConsumeEmailVerificationToken("expired"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("ConsumeEmailVerificationToken expired token = %v, want expired error", err)
	}
}

func TestSetAccountEmailClearsVerificationState(t *testing.T) {
	resetPostLimitStateForTest(t)

	verifiedAt := time.Now().Add(-time.Hour)
	mutex.Lock()
	accounts["acct-1"] = &Account{
		ID:              "acct-1",
		Created:         time.Now(),
		Email:           "old@example.com",
		EmailVerified:   true,
		EmailVerifiedAt: verifiedAt,
	}
	mutex.Unlock()

	if err := SetAccountEmail("acct-1", "new@example.com"); err != nil {
		t.Fatalf("SetAccountEmail returned error: %v", err)
	}

	mutex.Lock()
	acc := accounts["acct-1"]
	mutex.Unlock()
	if acc.Email != "new@example.com" {
		t.Fatalf("SetAccountEmail email = %q, want new@example.com", acc.Email)
	}
	if acc.EmailVerified {
		t.Fatal("SetAccountEmail left EmailVerified true")
	}
	if !acc.EmailVerifiedAt.IsZero() {
		t.Fatalf("SetAccountEmail EmailVerifiedAt = %v, want zero time", acc.EmailVerifiedAt)
	}
}
