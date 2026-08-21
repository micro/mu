package account

import (
	"testing"
	"time"

	"mu/internal/quota"

	"mu/internal/auth"
)

// Charging zero must succeed.
//
// Every content write — blog post, comment, reply, status, console note, app —
// is deliberately priced at zero, because it only touches this instance's own
// storage. quota.ConsumeQuota passed that zero to DeductCredits, which rejects any
// non-positive amount, and the centralised write gate turned the rejection into
// a 402 Payment Required. So the whole free half of the product was refused for
// want of credit nobody was being asked for.
//
// It stayed hidden because the two groups who could have found it never got
// there: admins skip the charge, and a new account was stopped by the 24-hour
// post gate before it reached this line. What was left was every ordinary
// established user, on every single write.
func TestFreeOperationsAreNotRefused(t *testing.T) {
	// The first account on an empty instance is bootstrapped to admin, and an
	// admin skips the charge — which is exactly the blind spot this test exists
	// to cover. So burn one, then test with the second.
	_ = auth.Create(&auth.Account{ID: "free-write-first", Name: "first", Secret: "x", Created: time.Now()})

	const id = "free-write-user"
	acc := &auth.Account{ID: id, Name: id, Secret: "x", Created: time.Now()}
	if err := auth.Create(acc); err != nil {
		t.Skipf("cannot create an account in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = auth.DeleteAccount(id)
		_ = auth.DeleteAccount("free-write-first")
	})
	if acc.Admin {
		t.Skip("test account was bootstrapped to admin; admins skip the charge")
	}

	free := []string{
		quota.OpBlogCreate, quota.OpBlogComment, quota.OpSocialPost,
		quota.OpSocialReply, quota.OpAppCreate,
	}
	for _, op := range free {
		if cost := quota.OperationCost(op); cost != 0 {
			t.Fatalf("%s is priced at %d, not free — update this test or the price", op, cost)
		}
		if err := quota.Charge(id, op, nil); err != nil {
			t.Fatalf("charging a free operation failed: %s: %v", op, err)
		}
	}

	// Free must not have become a bypass: a priced operation on an empty wallet
	// is still refused. There used to be a daily grant of credits to spend down
	// before this held, and there is not any more.
	quota.ResetAllowances()
	t.Cleanup(quota.ResetAllowances)
	if quota.OperationCost(quota.OpImageGenerate) > 0 {
		if err := quota.Charge(id, quota.OpImageGenerate, nil); err == nil {
			t.Fatal("a priced operation was allowed through on an empty wallet " +
				"with no allowance left")
		}
	}
}
