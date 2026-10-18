package billing

import (
	"mu/internal/group"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGroupBudgetSerializesConcurrentSpending(t *testing.T) {
	g, err := group.Create("budget-owner", "Budget test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(g.ID, g.Owner, "delete", "", "")
	owner := group.Scope(g.ID)
	if err := group.Configure(g.ID, g.Owner, 10, true); err != nil {
		t.Fatal(err)
	}
	if err := AddCredits(owner, 100, "test", nil); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			settle, e := reserveIncluded(owner, "group-test", 2)
			if e == nil {
				successes.Add(1)
				if e = settle(true); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 5 || GroupSpent(owner) != 10 {
		t.Fatalf("overspent cap: %d calls, %d credits", successes.Load(), GroupSpent(owner))
	}
	if Balance(owner) != 90 {
		t.Fatalf("wrong group balance: %d", Balance(owner))
	}
	if Balance(g.Owner) != 0 {
		t.Fatal("charged creator's personal wallet")
	}
}
func TestGroupReservationRefundRestoresBudget(t *testing.T) {
	g, _ := group.Create("refund-owner", "Refund", false)
	defer group.Change(g.ID, g.Owner, "delete", "", "")
	id := group.Scope(g.ID)
	_ = group.Configure(g.ID, g.Owner, 2, true)
	_ = AddCredits(id, 10, "test", nil)
	settle, err := reserveIncluded(id, "group-test", 2)
	if err != nil {
		t.Fatal(err)
	}
	if GroupSpent(id) != 2 {
		t.Fatal("pending reservation omitted")
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if GroupSpent(id) != 0 || Balance(id) != 10 {
		t.Fatal("failed operation consumed budget")
	}
}
