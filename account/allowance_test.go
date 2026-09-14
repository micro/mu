package account

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"mu/internal/data"
	"mu/internal/quota"
)

func TestIncludedBudgetPersistsAndDoesNotBecomeMoney(t *testing.T) {
	if err := quota.Load([]byte(`{"daily_credits":200,"operations":[]}`)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b, _ := os.ReadFile("../quota.json"); quota.Load(b) })
	const id = "included-budget"
	if err := AddCredits(id, 10, "test", nil); err != nil {
		t.Fatal(err)
	}
	if err := chargeIncluded(id, 198, "agent_run", nil); err != nil {
		t.Fatal(err)
	}
	if Balance(id) != 10 || IncludedToday(id) != 2 {
		t.Fatal("included usage changed the paid balance")
	}
	if err := chargeIncluded(id, 5, "web_search", nil); err != nil {
		t.Fatal(err)
	}
	if Balance(id) != 7 || IncludedToday(id) != 0 {
		t.Fatal("partial allowance was not used first")
	}
	b, err := data.LoadFile("transactions.json")
	if err != nil {
		t.Fatal(err)
	}
	withLedger(func(l *ledger) bool { json.Unmarshal(b, &transactions); return true })
	if IncludedToday(id) != 0 {
		t.Fatal("reloading grants another allowance")
	}
	next := withLedger(func(l *ledger) int { return includedToday(l, id, time.Now().UTC().Add(24*time.Hour)) })
	if next != 200 {
		t.Fatalf("next day budget = %d", next)
	}
	if err := DeductCredits(id, 8, "transfer", nil); err == nil {
		t.Fatal("allowance funded a transfer")
	}
}

func TestConcurrentIncludedChargesCannotOverspend(t *testing.T) {
	quota.Load([]byte(`{"daily_credits":200,"operations":[]}`))
	t.Cleanup(func() { b, _ := os.ReadFile("../quota.json"); quota.Load(b) })
	const id = "included-concurrent"
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = chargeIncluded(id, 20, "agent_run", nil) }()
	}
	wg.Wait()
	if IncludedToday(id) != 0 || Balance(id) != 0 {
		t.Fatal("daily allowance was overspent")
	}
	count := withLedger(func(l *ledger) int { return len(transactions[id]) })
	if count != 10 {
		t.Fatalf("charged %d operations, want 10", count)
	}
}
