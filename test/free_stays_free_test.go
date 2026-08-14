package test

// What quota.json calls free has to actually be free.
//
// The cost table on /pricing and /account renders straight from quota.json, so a
// row saying "free" is a promise made to whoever read it. Thirteen operations
// make that promise, and they are the ones that only touch this instance's own
// storage: posting, commenting, replying, creating an app, sending a message to
// somebody here, and the searches a provider does not bill us for.
//
// It is worth holding down because the pressure is all one way. Every change to
// billing in this repo has been about finding calls that were charged nowhere
// and charging them — the gateway, then the ten priced tools dispatched with no
// caller — and each of those made the charging path reach further. A free
// operation now goes through the same gate as a paid one; what stops it costing
// anything is one branch in ConsumeWith and one comparison in CheckQuota.
//
// The specific failure this guards against has already happened once, from the
// other direction: handing a zero to DeductCredits returns "amount must be
// positive", the write gate turned that into a 402, and every blog post,
// comment, reply, status and app was refused for want of credit nobody was
// asking for.

import (
	"testing"

	"mu/account"
	"mu/internal/auth"
	"mu/internal/quota"
)

// loadPrices reads the real quota.json, which main() does at boot and a test
// binary does not.
//
// Without it every operation falls back to the price an unpublished operation
// gets — 1 credit — so news_search, blog_create and eleven others look like
// they charge. That is the fallback working as designed and it made the first
// version of this test report thirteen defects that were not there. A test
// about what things cost has to load the file that says what things cost.
func loadPrices(t *testing.T) {
	t.Helper()
	if err := quota.LoadFromTree(); err != nil {
		t.Fatalf("cannot read quota.json: %v", err)
	}
	if quota.GetOperationCost("web_search") == 0 {
		t.Fatal("quota.json loaded but web_search is free, so nothing was read")
	}
}

// freeAccount is an ordinary established account: not an admin, not the
// instance's own agent, both of which skip the charge for their own reasons and
// would hide the thing being tested.
func freeAccount(t *testing.T) *auth.Account {
	t.Helper()
	loadPrices(t)
	acc := &auth.Account{ID: "free-stays-free", Name: "free-stays-free"}
	if existing, err := auth.GetAccount(acc.ID); err == nil {
		return existing
	}
	if err := auth.Create(acc); err != nil {
		t.Skipf("cannot create an account in this environment: %v", err)
	}
	t.Cleanup(func() { auth.DeleteAccount(acc.ID) }) //nolint:errcheck
	return acc
}

// TestAFreeOperationDoesNotMoveTheBalance — every operation priced at 0, run
// through the same charge the gateway makes, against a real balance.
func TestAFreeOperationDoesNotMoveTheBalance(t *testing.T) {
	acc := freeAccount(t)
	if acc.Admin || acc.Agent {
		t.Skip("this account is exempt for other reasons, so it proves nothing")
	}
	if err := account.AddCredits(acc.ID, 100, quota.OpTopup, nil); err != nil {
		t.Skipf("cannot fund a wallet here: %v", err)
	}
	before := quota.BalanceOf(acc.ID)

	var charged []string
	for _, p := range quotaPrices(t) {
		if p.cost != 0 {
			continue
		}
		// Exactly what internal/service/gateway.go does: ask, then charge.
		ok, _, cost, err := quota.CheckQuota(acc.ID, p.op)
		if err != nil {
			t.Errorf("%s is priced free and asking about it failed: %v", p.op, err)
			continue
		}
		if !ok {
			t.Errorf("%s is priced free and was refused — a free call must not depend "+
				"on a balance", p.op)
			continue
		}
		if cost != 0 {
			t.Errorf("%s is 0 in quota.json and the gate quoted %d", p.op, cost)
		}
		if err := quota.ConsumeWith(acc.ID, p.op, nil); err != nil {
			t.Errorf("%s is priced free and charging it failed: %v — this is the shape "+
				"that turned every blog post into a 402", p.op, err)
		}
		if now := quota.BalanceOf(acc.ID); now != before {
			charged = append(charged, p.op)
			before = now
		}
	}
	for _, op := range charged {
		t.Errorf("%s is published as free and took credits", op)
	}
}

// TestAFreeOperationWorksOnAnEmptyBalance — the promise is not "free once you
// have credits". A new account has none, and the free half of the product is
// what it can still do.
func TestAFreeOperationWorksOnAnEmptyBalance(t *testing.T) {
	loadPrices(t)
	const skint = "free-stays-free-skint"
	acc := &auth.Account{ID: skint, Name: skint}
	if _, err := auth.GetAccount(skint); err != nil {
		if err := auth.Create(acc); err != nil {
			t.Skipf("cannot create an account in this environment: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(skint) }) //nolint:errcheck
	}
	if quota.BalanceOf(skint) != 0 {
		t.Skip("this account has credits, so it cannot show what a broke one does")
	}

	for _, p := range quotaPrices(t) {
		if p.cost != 0 {
			continue
		}
		ok, _, _, err := quota.CheckQuota(skint, p.op)
		if err != nil || !ok {
			t.Errorf("%s is published as free and an account with 0 credits was "+
				"refused (ok=%v err=%v)", p.op, ok, err)
		}
	}
}

// TestEveryPublishedFreeRowIsFreeInQuota — the table and the gate read the same
// file, and this is the assertion that they agree about what "free" means.
func TestEveryPublishedFreeRowIsFreeInQuota(t *testing.T) {
	loadPrices(t)
	if len(account.Pricing()) == 0 {
		t.Fatal("the published price list is empty, so this asserts nothing")
	}
	for _, item := range account.Pricing() {
		if item.Cost != 0 {
			continue
		}
		if got := quota.GetOperationCost(item.Operation); got != 0 {
			t.Errorf("%s (%s) renders as free on /pricing and the gate charges %d",
				item.Operation, item.Description, got)
		}
	}
}
