package quota

// The front door: what an account with no money can do.
//
// Nothing grants credits at signup — AddCredits is called by the admin console
// and by Stripe, and by nothing else — so a new account has a balance of zero.
// The question is what that account can do, and the answer has been wrong twice.
//
// It was "nothing": the product had a price list, a top-up page, a ledger and no
// free tier, so the first question anybody asked on the web was answered with a
// link to a payment page.
//
// Then it was a daily grant of a hundred credits, spendable on anything priced,
// which fixed the symptom by paying the bill for you. Two mechanisms cancelling
// out, and a number a person had to understand before they could speak to their
// own agent — and it quietly zeroed the price on the three operations that reach
// a stranger, where service/sms says in as many words that the price is the
// control.
//
// Now: talking to the agent costs nothing, so there is no bill to pay and
// nothing to grant. Credits buy what a third party charges us for. These tests
// hold that shape.

import (
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

// A new account can use the free half of the product.
//
// Talking to the agent is not in the price list at all — the agent is not a
// service, so it has no operation and no price. What this holds is the rule
// underneath: an operation that costs this instance nothing is not refused for
// want of credit, which is what made a zero balance useless before.
func TestANewAccountCanUseWhatIsFree(t *testing.T) {
	const id = "door-new"
	charging(t, id)

	if BalanceOf(id) != 0 {
		t.Fatal("this account is supposed to have no money")
	}
	if c := OperationCost(OpNewsSearch); c != 0 {
		t.Fatalf("reading the news costs %d credits — it touches nothing we pay "+
			"for, and charging for it taxes the behaviour the product wants", c)
	}
	ok, _, cost, err := CheckQuota(id, OpNewsSearch)
	if !ok {
		t.Fatalf("an account with no credits was refused a free operation "+
			"(costs %d): %v", cost, err)
	}
}

// And is refused the things that cost somebody money.
//
// The other half, and the half a free tier keeps breaking. An account with
// nothing in it must not be able to run a web search, generate an image or send
// a text — those are real money to this instance the moment they happen.
func TestANewAccountIsRefusedWhatCostsMoney(t *testing.T) {
	const id = "door-broke"
	charging(t, id)

	for _, op := range []string{OpWebSearch, OpImageGenerate, OpSMSSend, OpMailSend} {
		if OperationCost(op) == 0 {
			t.Errorf("%s is free — it is a third party's bill and should not be", op)
			continue
		}
		if ok, _, _, _ := CheckQuota(id, op); ok {
			t.Errorf("%s was allowed on an empty balance", op)
		}
	}
}

// Nothing hands out credits behind the balance's back.
//
// The daily grant is gone and must not come back by another name: an empty
// account is empty, and the only way past a price is to pay it. This is the
// test that would have caught the allowance quietly making the first five texts
// free.
func TestNothingIsGrantedForFree(t *testing.T) {
	const id = "door-grant"
	charging(t, id)

	// A run of calls that would have fitted inside the old hundred-credit day.
	for i := 0; i < 20; i++ {
		if ok, _, _, _ := CheckQuota(id, OpWebSearch); ok {
			t.Fatalf("call %d on an empty balance was allowed — something is "+
				"granting credits again", i+1)
		}
	}
	if BalanceOf(id) != 0 {
		t.Error("the balance moved without anybody paying")
	}
}

// A free operation is free, and does not become a way past a price.
func TestAFreeOperationStaysFree(t *testing.T) {
	const id = "door-free"
	charging(t, id)

	if ok, _, cost, err := CheckQuota(id, OpNewsSearch); !ok || cost != 0 {
		t.Errorf("a free operation was refused or priced: ok=%v cost=%d err=%v", ok, cost, err)
	}
	if ok, _, _, _ := CheckQuota(id, OpImageGenerate); ok {
		t.Error("a priced operation rode in on a free one")
	}
}
