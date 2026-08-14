package wallet

import (
	"math/big"
	"testing"
)

// TestSpendLimits verifies agent-initiated payments are bounded per call and
// per day, so a misled agent cannot drain the wallet even though the source
// account is correctly bound.
func TestSpendLimits(t *testing.T) {
	acct := "spend-test-acct" // unique so the in-process ledger doesn't collide

	// Under the per-call cap ($1 = 1_000_000 atomic): allowed.
	if err := checkAndRecordSpend(acct, big.NewInt(400_000)); err != nil {
		t.Fatalf("in-cap payment rejected: %v", err)
	}

	// Over the per-call cap: rejected, and not recorded.
	if err := checkAndRecordSpend(acct, big.NewInt(1_500_000)); err == nil {
		t.Fatal("payment over per-call cap should be rejected")
	}

	// A zero/negative amount is rejected.
	if err := checkAndRecordSpend(acct, big.NewInt(0)); err == nil {
		t.Fatal("zero payment should be rejected")
	}

	// Accumulate to the daily cap ($10 = 10_000_000). Already spent 400_000;
	// add 9_600_000 in $1 chunks to reach exactly the cap.
	for i := 0; i < 9; i++ {
		if err := checkAndRecordSpend(acct, big.NewInt(1_000_000)); err != nil {
			t.Fatalf("chunk %d within daily cap rejected: %v", i, err)
		}
	}
	if err := checkAndRecordSpend(acct, big.NewInt(600_000)); err != nil {
		t.Fatalf("final chunk to reach cap rejected: %v", err)
	}

	// Now at the daily cap; any further spend is rejected.
	if err := checkAndRecordSpend(acct, big.NewInt(1)); err == nil {
		t.Fatal("payment over daily cap should be rejected")
	}

	// A different account has its own budget.
	if err := checkAndRecordSpend("other-acct", big.NewInt(500_000)); err != nil {
		t.Fatalf("independent account rejected: %v", err)
	}
}
