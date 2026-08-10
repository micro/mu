package wallet

// The invariants that hold when money moves.
//
// Everything here is something that broke, or could have, in one day:
//
//   - a transfer resolved its recipient by map order and paid the wrong account
//   - a receipt recorded an opaque id, so a wrong answer looked like a right one
//   - a free operation was floored to one credit and billed
//   - asking what something costs failed when you could not afford it
//
// None of them were caught by a test, and all of them are cheap to state. What
// they have in common is that the code was *almost* right: the balances still
// added up, the transfer still completed, the gate still gated. Money bugs do
// not usually look like errors.

import (
	"context"
	"os"
	"testing"

	"mu/internal/quota"

	"mu/internal/auth"
	"mu/internal/service"
)

// withAccount is how identity reaches a service: from the call context, never
// from a field in the request. See internal/service/identity.go.
func withAccount(t *testing.T, id string) context.Context {
	t.Helper()
	return service.WithAccount(context.Background(), id)
}

func moneyHome(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mu-money")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })

	mutex.Lock()
	wallets = map[string]*Wallet{}
	transactions = map[string][]*Transaction{}
	mutex.Unlock()
}

func account(t *testing.T, id, name string) {
	t.Helper()
	if _, err := auth.GetAccount(id); err == nil {
		return
	}
	if err := auth.Create(&auth.Account{ID: id, Name: name, Secret: "s"}); err != nil {
		t.Fatal(err)
	}
}

// Credits are conserved. A transfer moves them; it never creates or destroys
// any, whatever else it gets wrong.
func TestATransferConservesCredits(t *testing.T) {
	moneyHome(t)
	account(t, "payer", "Payer")
	account(t, "payee", "Payee")
	AddCredits("payer", 100, "test", nil)

	before := GetBalance("payer") + GetBalance("payee")
	if err := TransferCredits("payer", "payee", 30); err != nil {
		t.Fatal(err)
	}
	after := GetBalance("payer") + GetBalance("payee")

	if before != after {
		t.Errorf("credits were created or destroyed: %d before, %d after", before, after)
	}
	if got := GetBalance("payer"); got != 70 {
		t.Errorf("payer has %d, want 70", got)
	}
	if got := GetBalance("payee"); got != 30 {
		t.Errorf("payee has %d, want 30", got)
	}
}

// Both sides of a transfer are written, and each says who the other was by
// name. The receipt that read "Transfer to 3834" is why: an id alone is
// unreadable, and an id alone made a misrouted transfer indistinguishable from
// a correct one.
func TestATransferIsRecordedOnBothSidesWithNames(t *testing.T) {
	moneyHome(t)
	account(t, "sender", "Sender Display")
	account(t, "getter", "Getter Display")
	AddCredits("sender", 50, "test", nil)

	if err := TransferCredits("sender", "getter", 20); err != nil {
		t.Fatal(err)
	}

	out := GetTransactions("sender", 10)
	if len(out) == 0 {
		t.Fatal("the sender has no record of the transfer")
	}
	sent := out[0]
	if sent.Amount != -20 {
		t.Errorf("the sender's side records %d, want -20", sent.Amount)
	}
	if sent.Metadata["to"] != "getter" {
		t.Errorf("the sender's receipt does not name the account credited: %v", sent.Metadata)
	}
	if sent.Metadata["to_name"] != "Getter Display" {
		t.Errorf("the sender's receipt has no readable name: %v", sent.Metadata)
	}

	in := GetTransactions("getter", 10)
	if len(in) == 0 {
		t.Fatal("the recipient has no record of the transfer")
	}
	if in[0].Amount != 20 || in[0].Metadata["from"] != "sender" {
		t.Errorf("the recipient's side is wrong: %+v", in[0])
	}
}

// You cannot send what you do not have, and a refused transfer leaves both
// balances untouched.
func TestAnUnaffordableTransferChangesNothing(t *testing.T) {
	moneyHome(t)
	account(t, "poor", "Poor")
	account(t, "rich", "Rich")
	AddCredits("poor", 5, "test", nil)

	if err := TransferCredits("poor", "rich", 50); err == nil {
		t.Fatal("a transfer larger than the balance was allowed")
	}
	if got := GetBalance("poor"); got != 5 {
		t.Errorf("the sender's balance moved on a refused transfer: %d", got)
	}
	if got := GetBalance("rich"); got != 0 {
		t.Errorf("the recipient was credited by a refused transfer: %d", got)
	}
}

func TestTransfersRefuseNonsense(t *testing.T) {
	moneyHome(t)
	account(t, "solo", "Solo")
	AddCredits("solo", 100, "test", nil)

	for _, c := range []struct {
		name     string
		from, to string
		amount   int
	}{
		{"to yourself", "solo", "solo", 10},
		{"zero", "solo", "other", 0},
		{"negative", "solo", "other", -10},
		{"over the daily cap", "solo", "other", DailyTransferCap + 1},
	} {
		if err := TransferCredits(c.from, c.to, c.amount); err == nil {
			t.Errorf("%s was allowed", c.name)
		}
	}
	if got := GetBalance("solo"); got != 100 {
		t.Errorf("a refused transfer moved credits: %d", got)
	}
}

// A free operation is free at every layer that could charge for it.
func TestNothingChargesForAFreeOperation(t *testing.T) {
	for _, free := range []string{quota.OpNewsSearch, quota.OpQuranSearch, quota.OpWebFetch, quota.OpVideoSearch} {
		if c := quota.GetOperationCost(free); c != 0 {
			t.Errorf("%s costs %d — this test is about the ones priced at zero", free, c)
		}
		if quota.Metered(free) {
			t.Errorf("%s reads as metered", free)
		}
		if reqs := BuildPaymentRequirements(free, "https://x.test/mcp"); len(reqs) != 0 {
			t.Errorf("%s was given a payment challenge for %s", free, reqs[0].MaxAmountRequired)
		}
	}
}
