package billing

import (
	"context"
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

func TestGroupDeletionPreservesFundsAndPendingRefunds(t *testing.T) {
	g, err := group.Create("deletion-owner", "Funded", false)
	if err != nil {
		t.Fatal(err)
	}
	id := group.Scope(g.ID)
	if err := AddCredits(id, 10, "test", nil); err != nil {
		t.Fatal(err)
	}
	if err := group.Change(g.ID, g.Owner, "delete", "", ""); err == nil {
		t.Fatal("deleted funded group")
	}
	if err := group.Forget(g.Owner); err == nil {
		t.Fatal("account cleanup orphaned funded group")
	}
	if err := EndSubscription(context.Background(), g.Owner); err == nil {
		t.Fatal("account deletion preflight accepted funded group")
	}
	settle, err := reserveIncluded(id, "test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if Balance(id) != 0 {
		t.Fatal("expected full balance reserved")
	}
	if err := group.Change(g.ID, g.Owner, "delete", "", ""); err == nil {
		t.Fatal("deleted group before refund could settle")
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if Balance(id) != 10 {
		t.Fatal("refund lost")
	}
	settle, err = reserveIncluded(id, "test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(true); err != nil {
		t.Fatal(err)
	}
	if err := group.Change(g.ID, g.Owner, "delete", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := AddCredits(id, 1, "test", nil); err == nil {
		t.Fatal("credited deleted group")
	}
}

func TestGroupFundingRacesDeletion(t *testing.T) {
	for range 20 {
		g, err := group.Create("racing-owner", "Race", false)
		if err != nil {
			t.Fatal(err)
		}
		payer := "payer-" + g.ID
		if err := AddCredits(payer, 1, "test", nil); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var funding, deletion error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; funding = TransferCredits(payer, group.Scope(g.ID), 1) }()
		go func() { defer wg.Done(); <-start; deletion = group.Change(g.ID, g.Owner, "delete", "", "") }()
		close(start)
		wg.Wait()
		_, exists := group.Details(g.ID)
		if funding == nil {
			if deletion == nil || !exists || Balance(group.Scope(g.ID)) != 1 {
				t.Fatal("funded group disappeared")
			}
			if err := TransferCredits(group.Scope(g.ID), payer, 1); err != nil {
				t.Fatal(err)
			}
			if err := group.Change(g.ID, g.Owner, "delete", "", ""); err != nil {
				t.Fatal(err)
			}
		} else if deletion != nil || exists || Balance(payer) != 1 {
			t.Fatal("failed transfer lost payer credit")
		}
	}
}
