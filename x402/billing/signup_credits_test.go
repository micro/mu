package billing

import (
	"mu/internal/auth"
	"mu/internal/quota"
	"testing"
)

func TestSignupAndDailyGrantsDisabled(t *testing.T) {
	acc := subscriptionFixture(t)
	copy := *acc
	copy.SignupCredits = 100 // legacy eligibility must not grant on login
	auth.SetAccountForTest(&copy)
	for range 2 {
		GrantSignup(acc.ID)
		RetrySignup(acc.ID)
	}
	if SignupRemaining(acc.ID) != 0 || IncludedToday(acc.ID) != 0 || quota.Available(acc.ID) != 0 {
		t.Fatal("unpaid account received free credit")
	}
	if _, err := reserveIncluded(acc.ID, quota.OpAgentRun, 1); err == nil {
		t.Fatal("unfunded operation allowed")
	}
	if err := AddCredits(acc.ID, 100, quota.OpTopup, nil); err != nil {
		t.Fatal(err)
	}
	GrantSignup(acc.ID)
	if Balance(acc.ID) != 100 {
		t.Fatal("purchased balance changed")
	}
	settle, err := reserveIncluded(acc.ID, quota.OpAgentRun, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if Balance(acc.ID) != 100 {
		t.Fatal("purchased credits not refunded")
	}
}
