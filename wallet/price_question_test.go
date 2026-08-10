package wallet

// Asking the price is not spending. This failed with "this costs 2 credits and
// your balance is 0" — so the one caller who most needs a price, somebody
// deciding whether to top up, was the one who could not ask.

import (
	"context"
	"os"
	"testing"

	"mu/internal/quota"

	"mu/internal/auth"
	"mu/internal/service"
)

func TestYouCanAskThePriceWithNoCredits(t *testing.T) {
	dir, err := os.MkdirTemp("", "mu-price")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })

	if _, err := auth.GetAccount("broke"); err != nil {
		if err := auth.Create(&auth.Account{ID: "broke", Name: "Broke", Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}

	var rsp CheckResponse
	ctx := service.WithAccount(context.Background(), "broke")
	if err := (Credits{}).Check(ctx, &CheckRequest{Operation: quota.OpWebSearch}, &rsp); err != nil {
		t.Fatalf("asking the price of a tool failed: %v", err)
	}
	if rsp.Price != quota.GetOperationCost(quota.OpWebSearch) {
		t.Errorf("price is %d, want %d", rsp.Price, quota.GetOperationCost(quota.OpWebSearch))
	}
	// Allowed may be true on an instance with payments off; what must not
	// happen is the call failing.
}
