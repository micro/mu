package account

import (
	"mu/internal/auth"
	"mu/x402/billing"
	"testing"
)

func TestGoogleSignupGetsAllowanceOnlyOnce(t *testing.T) {
	t.Setenv("ADMIN", "operator")
	t.Setenv("STRIPE_SECRET_KEY", "test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "test")
	info := &googleUser{Email: "launchgrant@example.test", Name: "Launch grant"}
	acc := findOrCreateGoogleAccount(info)
	if acc == nil {
		t.Fatal("account not created")
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(acc.ID) })
	if billing.SignupRemaining(acc.ID) != 100 {
		t.Fatal("no signup allowance")
	}
	if again := findOrCreateGoogleAccount(info); again == nil || again.ID != acc.ID || billing.SignupRemaining(acc.ID) != 100 {
		t.Fatal("returning Google login reissued allowance")
	}
	other := &auth.Account{ID: "nograntread", Secret: "test-password"}
	if err := auth.Create(other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(other.ID) })
	_ = billing.CreditsOf(other.ID)
	if billing.SignupRemaining(other.ID) != 0 {
		t.Fatal("reading an existing account grants credits")
	}
}
