package quota

// The front door.
//
// Nothing granted an account credits at signup — AddCredits is called by the
// admin console and by Stripe, and by nothing else — so a new account had a
// balance of zero and CheckQuota refused it everything. The product had a price
// list, a top-up page, a ledger and no free tier at all: the first question
// anybody asked on the web was answered with a link to a payment page.
//
// daily_quota is the floor. It was already in quota.json, already parsed,
// already overridable, and read by nothing.

import (
	"sync"
	"testing"

	"mu/internal/auth"
)

// charging stands up a person with no money on an instance that can bill.
func charging(t *testing.T, id string) {
	t.Helper()
	if err := LoadFromTree(); err != nil {
		t.Fatalf("could not load quota.json: %v", err)
	}
	prev := Enabled
	Enabled = func() bool { return true }
	t.Cleanup(func() { Enabled = prev })

	auth.SetAccountForTest(&auth.Account{ID: id, Name: id})
	t.Cleanup(func() { auth.RemoveAccountForTest(id) })

	ResetAllowances()
	t.Cleanup(ResetAllowances)
}

// A new account can ask a question. This is the bug.
func TestANewAccountCanUseTheProduct(t *testing.T) {
	const id = "free-new"
	charging(t, id)

	if BalanceOf(id) != 0 {
		t.Fatal("this account is supposed to have no money")
	}
	ok, _, cost, err := CheckQuota(id, OpAgentQuery)
	if !ok {
		t.Fatalf("an account with no credits was refused an agent question "+
			"(costs %d): %v — nothing grants credits at signup, so this is every "+
			"new account's first action", cost, err)
	}
}

// The allowance is spent, not merely offered — or it would be infinite.
func TestTheAllowanceRunsOut(t *testing.T) {
	const id = "free-spend"
	charging(t, id)

	cost := OperationCost(OpAgentQuery)
	if cost <= 0 {
		t.Fatal("agent_query is free, so this proves nothing")
	}
	runs := 0
	for {
		ok, _, _, _ := CheckQuota(id, OpAgentQuery)
		if !ok {
			break
		}
		if err := ConsumeQuota(id, OpAgentQuery); err != nil {
			t.Fatalf("run %d: %v", runs, err)
		}
		runs++
		if runs > DailyQuota+10 {
			t.Fatal("the allowance never ran out, so it is not being spent")
		}
	}
	if runs == 0 {
		t.Fatal("not one call fitted in the allowance")
	}
	// Whole calls only, so the last one that fits is the last one charged.
	if want := DailyQuota / cost; runs != want {
		t.Errorf("%d runs on a %d-credit allowance at %d each, want %d",
			runs, DailyQuota, cost, want)
	}
	if left := FreeCreditsLeft(id); left >= cost {
		t.Errorf("%d credits left but the call was refused", left)
	}
}

// Nothing is taken from the balance while the allowance covers it.
func TestTheAllowanceIsSpentBeforeTheBalance(t *testing.T) {
	const id = "free-first"
	charging(t, id)

	var deducted int
	prev := Deduct
	Deduct = func(_, _ string, amount int, _ map[string]interface{}) error {
		deducted += amount
		return nil
	}
	t.Cleanup(func() { Deduct = prev })

	if err := ConsumeQuota(id, OpWebSearch); err != nil {
		t.Fatal(err)
	}
	if deducted != 0 {
		t.Errorf("%d credits came off the balance for a call the allowance covered", deducted)
	}
	if used := FreeCreditsUsed(id); used != OperationCost(OpWebSearch) {
		t.Errorf("the allowance recorded %d for a %d-credit call",
			used, OperationCost(OpWebSearch))
	}
}

// Past the allowance, the balance pays — which is the half that keeps this a
// business rather than a giveaway.
func TestPastTheAllowanceTheBalancePays(t *testing.T) {
	const id = "free-then-paid"
	charging(t, id)

	var deducted int
	prev := Deduct
	Deduct = func(_, _ string, amount int, _ map[string]interface{}) error {
		deducted += amount
		return nil
	}
	t.Cleanup(func() { Deduct = prev })

	// Use it all up on something, then buy one more.
	for FreeCreditsLeft(id) > 0 && SpendFreeCredits(id, 1) {
	}
	if err := ConsumeQuota(id, OpWebSearch); err != nil {
		t.Fatal(err)
	}
	if want := OperationCost(OpWebSearch); deducted != want {
		t.Errorf("charged %d past the allowance, want %d", deducted, want)
	}
}

// A call costing more than what is left is paid for in full from the balance,
// not part-funded from the remainder.
//
// Part-funding would leave somebody paying 5 for a run the price list says is
// 7, which is a receipt nobody can check.
func TestAnOversizedCallDoesNotDrainTheRemainder(t *testing.T) {
	const id = "free-partial"
	charging(t, id)

	cost := OperationCost(OpAgentQuery)
	if cost < 2 {
		t.Skip("agent_query is too cheap to leave a remainder")
	}
	// Leave one credit less than the call needs.
	for FreeCreditsLeft(id) > cost-1 {
		if !SpendFreeCredits(id, 1) {
			t.Fatal("could not set up the remainder")
		}
	}
	before := FreeCreditsUsed(id)

	var deducted int
	prev := Deduct
	Deduct = func(_, _ string, amount int, _ map[string]interface{}) error {
		deducted += amount
		return nil
	}
	t.Cleanup(func() { Deduct = prev })

	if err := ConsumeQuota(id, OpAgentQuery); err != nil {
		t.Fatal(err)
	}
	if deducted != cost {
		t.Errorf("charged %d, want the whole %d", deducted, cost)
	}
	if after := FreeCreditsUsed(id); after != before {
		t.Errorf("the allowance moved from %d to %d for a call it could not "+
			"cover — the remainder was drained", before, after)
	}
}

// A free operation costs neither credits nor allowance. Reading and listing are
// free, and making them eat a paid allowance would tax exactly the behaviour
// the price list is written to encourage.
func TestAFreeOperationDoesNotSpendTheAllowance(t *testing.T) {
	const id = "free-zero"
	charging(t, id)

	if OperationCost(OpNewsSearch) != 0 {
		t.Skip("news_search is priced, so this proves nothing")
	}
	if err := ConsumeQuota(id, OpNewsSearch); err != nil {
		t.Fatal(err)
	}
	if used := FreeCreditsUsed(id); used != 0 {
		t.Errorf("a free call spent %d of the allowance", used)
	}
}

// Two calls landing together cannot both take the last credit.
func TestTheAllowanceIsNotOverspentUnderLoad(t *testing.T) {
	const id = "free-race"
	charging(t, id)

	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < DailyQuota*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if SpendFreeCredits(id, 1) {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != DailyQuota {
		t.Errorf("%d calls got a credit out of an allowance of %d", won, DailyQuota)
	}
	if left := FreeCreditsLeft(id); left != 0 {
		t.Errorf("%d credits left after the allowance was drained", left)
	}
}

// An operator can turn it off, for an instance where everything is paid for
// from the first call.
func TestTheAllowanceCanBeTurnedOff(t *testing.T) {
	const id = "free-off"
	charging(t, id)

	prev := DailyQuota
	DailyQuota = 0
	t.Cleanup(func() { DailyQuota = prev })

	if FreeCreditsLeft(id) != 0 {
		t.Error("an allowance of zero still hands out credits")
	}
	if SpendFreeCredits(id, 1) {
		t.Error("an allowance of zero was spent")
	}
	if ok, _, _, _ := CheckQuota(id, OpWebSearch); ok {
		t.Error("an account with no credits was allowed a metered call with the " +
			"allowance turned off")
	}
}

// One account's allowance is its own.
func TestAllowancesAreNotShared(t *testing.T) {
	const mine, theirs = "free-mine", "free-theirs"
	charging(t, mine)
	auth.SetAccountForTest(&auth.Account{ID: theirs, Name: theirs})
	t.Cleanup(func() { auth.RemoveAccountForTest(theirs) })

	for FreeCreditsLeft(mine) > 0 && SpendFreeCredits(mine, 1) {
	}
	if FreeCreditsLeft(theirs) != DailyQuota {
		t.Errorf("one account spending its allowance left another with %d of %d",
			FreeCreditsLeft(theirs), DailyQuota)
	}
}

// The per-operation allowance and the account's daily one are different things
// counted in the same map, so neither may read the other's tally.
func TestThePerOperationCountIsNotTheCreditCount(t *testing.T) {
	const id = "free-separate"
	charging(t, id)

	Done(id, OpWebSearch)
	Done(id, OpWebSearch)
	if used := FreeCreditsUsed(id); used != 0 {
		t.Errorf("two calls moved the credit allowance to %d — the counters are "+
			"sharing a key", used)
	}
	SpendFreeCredits(id, 3)
	if n := UsedToday(id, OpWebSearch); n != 2 {
		t.Errorf("spending credits changed the call count to %d, want 2", n)
	}
}
