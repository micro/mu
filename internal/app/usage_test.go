package app

import (
	"testing"
	"time"
)

func TestAccountCostWindowAndDeletion(t *testing.T) {
	usageMu.Lock()
	old := accountCosts
	accountCosts = map[string]map[string]DailyCost{}
	usageMu.Unlock()
	t.Cleanup(func() { usageMu.Lock(); accountCosts = old; usageMu.Unlock() })
	RecordUsage("model", "agent", 2.5, map[string]any{"account": "cost-owner"})
	RecordUsage("model", "agent", 1.5, map[string]any{"account": "cost-owner"})
	RecordUsage("model", "agent", 9, map[string]any{"account": "other-owner"})
	RecordUsage("model", "agent", 100, nil)
	usageMu.Lock()
	day := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	accountCosts["cost-owner"][day] = DailyCost{Day: day, Calls: 1, CostCents: 500}
	usageMu.Unlock()
	got := AccountCosts()
	if got["cost-owner"].CostCents != 4 || got["cost-owner"].Calls != 2 || got["other-owner"].CostCents != 9 {
		t.Fatalf("wrong account/window totals: %+v", got)
	}
	ForgetAccountCosts("cost-owner")
	if _, ok := AccountCosts()["cost-owner"]; ok {
		t.Fatal("deleted account kept attributed costs")
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	for _, record := range usageRecords {
		if record.Details["account"] == "cost-owner" {
			t.Fatal("deleted identity retained in recent log")
		}
	}
}
