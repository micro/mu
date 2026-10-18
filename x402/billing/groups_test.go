package billing

import (
	"context"
	"mu/internal/auth"
	"mu/internal/group"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestAccountDeletionRejectsFundingAfterPreflight(t *testing.T) {
	acc := subscriptionFixture(t)
	g, err := group.Create(acc.ID, "Late funding", false)
	if err != nil {
		t.Fatal(err)
	}
	oldPrepare, oldHooks := auth.AccountDeletePrepare, auth.AccountDeleteHooks
	auth.AccountDeletePrepare, auth.AccountDeleteHooks = []func(string) error{group.Forget}, nil
	t.Cleanup(func() { auth.AccountDeletePrepare, auth.AccountDeleteHooks = oldPrepare, oldHooks })
	if err := EndSubscription(context.Background(), acc.ID); err != nil {
		t.Fatal(err)
	}
	// A payer arrives after billing preflight, before account deletion.
	if err := AddCredits(group.Scope(g.ID), 1, "late funding", nil); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteAccount(acc.ID); err == nil {
		t.Fatal("deleted account after late group funding")
	}
	if _, err := auth.GetAccount(acc.ID); err != nil {
		t.Fatal("owner lost access")
	}
	if !group.Member(g.ID, acc.ID) {
		t.Fatal("owner lost group membership")
	}
	if err := TransferCredits(group.Scope(g.ID), acc.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteAccount(acc.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := group.Details(g.ID); ok {
		t.Fatal("group removal was deferred until after account deletion")
	}
	if err := AddCredits(group.Scope(g.ID), 1, "late funding", nil); err == nil {
		t.Fatal("funded a retired group")
	}
}

func TestAccountDeletionValidatesAllGroupsBeforeCancellingPlans(t *testing.T) {
	acc := subscriptionFixture(t)
	for _, name := range []string{"First", "Second"} {
		g, err := group.Create(acc.ID, name, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			group.ProtectBilling(g.ID, time.Time{})
			withLedger(func(l *ledger) bool { delete(balances, group.Scope(g.ID)); return true })
			group.Change(g.ID, acc.ID, "delete", "", "")
		})
	}
	gs, _, err := group.List(acc.ID)
	if err != nil || len(gs) != 2 {
		t.Fatal("missing groups", err)
	}
	first := group.Scope(gs[0].ID)
	subscriptions[first] = subscription{ID: "sub_group", Status: "active"}
	if err := AddCredits(group.Scope(gs[1].ID), 1, "test", nil); err != nil {
		t.Fatal(err)
	}
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("changed subscription before checking every group")
		return nil, nil
	})}
	if err := EndSubscription(context.Background(), acc.ID); err == nil {
		t.Fatal("accepted funded group")
	}
	if subscriptions[first].Status != "active" {
		t.Fatal("cancelled another group's plan")
	}
}
