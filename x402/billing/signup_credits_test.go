package billing

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/dir"
	"mu/internal/quota"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSignupAllowanceLifecycle(t *testing.T) {
	t.Setenv("ADMIN", "operator")
	acc := signupFixture(t)
	for range 2 {
		if err := GrantSignup(acc.ID); err != nil {
			t.Fatal(err)
		}
	}
	if SignupRemaining(acc.ID) != 100 || Balance(acc.ID) != 0 || Paid(acc.ID) {
		t.Fatal("signup credit must be granted once, not money or paid trust")
	}
	if err := TransferCredits(acc.ID, "recipient", 1); err == nil {
		t.Fatal("signup credit transferred")
	}
	daily := IncludedToday(acc.ID)
	settle, err := reserveIncluded(acc.ID, quota.OpAgentRun, daily+30)
	if err != nil {
		t.Fatal(err)
	}
	if SignupRemaining(acc.ID) != 70 {
		t.Fatal("signup credit was not reserved")
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if SignupRemaining(acc.ID) != 100 || IncludedToday(acc.ID) != daily {
		t.Fatal("refund lost or duplicated allowance")
	}
	settle, err = reserveIncluded(acc.ID, quota.OpAgentRun, daily+40)
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(true); err != nil {
		t.Fatal(err)
	}
	// JSON reload is the restart representation, including numeric metadata.
	withLedger(func(l *ledger) bool {
		var transactionsForReload []*Transaction
		b, _ := json.Marshal(transactions[acc.ID])
		if err := json.Unmarshal(b, &transactionsForReload); err != nil {
			t.Fatal(err)
		}
		transactions[acc.ID] = transactionsForReload
		return true
	})
	if err := GrantSignup(acc.ID); err != nil {
		t.Fatal(err)
	}
	if SignupRemaining(acc.ID) != 60 {
		t.Fatal("restart reissued credit")
	}
	var wg sync.WaitGroup
	var successes int
	var mu sync.Mutex
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done, err := reserveIncluded(acc.ID, quota.OpAgentRun, 60)
			if err == nil {
				if err := done(true); err != nil {
					t.Error(err)
				}
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 || SignupRemaining(acc.ID) != 0 {
		t.Fatal("concurrent calls overspent signup credit")
	}
}

func TestSignupGrantPreservesLegacyBalance(t *testing.T) {
	t.Setenv("ADMIN", "operator")
	acc := signupFixture(t)
	if err := AddCredits(acc.ID, 100, OpWelcome, nil); err != nil {
		t.Fatal(err)
	}
	if err := GrantSignup(acc.ID); err != nil {
		t.Fatal(err)
	}
	if Balance(acc.ID) != 100 || SignupRemaining(acc.ID) != 0 {
		t.Fatal("legacy gift duplicated or changed")
	}
}

func signupFixture(t *testing.T) *auth.Account {
	t.Helper()
	acc := subscriptionFixture(t)
	copy := *acc
	copy.SignupCredits = SignupCredits
	auth.SetAccountForTest(&copy)
	return &copy
}

func TestSignupCreditRetriesOnLoginAfterWriteFailure(t *testing.T) {
	t.Setenv("ADMIN", "operator")
	_ = subscriptionFixture(t)
	acc := &auth.Account{ID: "signupretry", Secret: "test-password", SignupCredits: SignupCredits}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(acc.ID) })
	file := filepath.Join(dir.Data(), "transactions.json")
	if err := data.SaveJSON("transactions.json", transactions); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, file+".backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	restore := func() { once.Do(func() { os.Remove(file); os.Rename(file+".backup", file) }) }
	t.Cleanup(restore)
	if err := GrantSignup(acc.ID); err == nil {
		t.Fatal("expected ledger write failure")
	}
	if SignupRemaining(acc.ID) != 0 {
		t.Fatal("failed grant remained in memory")
	}
	restore()
	for range 2 {
		RetrySignup(acc.ID)
		if SignupRemaining(acc.ID) != SignupCredits {
			t.Fatal("retry failed to grant allowance")
		}
	}
}
