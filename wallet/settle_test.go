package wallet

// A payment must land whether or not the webhook ever arrives.
//
// Everything hung on one: the card was charged, Stripe called us, and that call
// was the only thing that credited an account or recorded a plan. If it was not
// configured, or its secret was wrong, or its event list was missing the one
// that mattered, the money moved and nothing here did — no credits, no plan, no
// error, nothing in the log to notice. And it is not a subscription problem: a
// one-off top-up hung on exactly the same thread.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
)

func settler(t *testing.T, id string) {
	t.Helper()
	auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}) //nolint:errcheck
	if _, err := auth.GetAccount(id); err != nil {
		t.Skipf("cannot create an account here: %v", err)
	}
	t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
}

func TestASettledSessionCreditsAndRecordsThePlan(t *testing.T) {
	const owner = "settle-paid"
	settler(t, owner)
	before := GetBalance(owner)

	settleSession(checkoutSession{
		ID: "cs_settle_1", PaymentStatus: "paid", Customer: "cus_x",
		AmountTotal: 2000, UserID: owner, Credits: "2000", PlanID: "pro",
	}, "test")

	if got := GetBalance(owner); got != before+2000 {
		t.Errorf("balance is %d, want %d", got, before+2000)
	}
	acc, _ := auth.GetAccount(owner)
	if acc.Plan != "pro" {
		t.Errorf("plan is %q, want pro", acc.Plan)
	}
	if acc.Customer != "cus_x" {
		t.Errorf("customer is %q, want cus_x", acc.Customer)
	}
}

// The whole point of one settle function: two routes, one effect.
func TestTheSameSessionSettlesOnlyOnce(t *testing.T) {
	const owner = "settle-twice"
	settler(t, owner)
	before := GetBalance(owner)

	s := checkoutSession{
		ID: "cs_settle_2", PaymentStatus: "paid",
		AmountTotal: 500, UserID: owner, Credits: "500",
	}
	settleSession(s, "webhook")
	settleSession(s, "return from checkout")

	if got := GetBalance(owner); got != before+500 {
		t.Errorf("balance is %d, want %d — the session was applied twice, which is "+
			"what a second settling route would cost if it were not deduped",
			got, before+500)
	}
}

// Nothing lands until Stripe says it was paid.
func TestAnUnpaidSessionChangesNothing(t *testing.T) {
	const owner = "settle-unpaid"
	settler(t, owner)
	before := GetBalance(owner)

	settleSession(checkoutSession{
		ID: "cs_settle_3", PaymentStatus: "unpaid",
		UserID: owner, Credits: "5000", PlanID: "scale",
	}, "test")

	if got := GetBalance(owner); got != before {
		t.Errorf("an unpaid session credited %d", got-before)
	}
	if acc, _ := auth.GetAccount(owner); acc.Plan != "" {
		t.Errorf("an unpaid session set the plan to %q", acc.Plan)
	}
}

// A session id is not a secret — it arrives in a URL and sits in a browser
// history — so the return path has to check whose purchase it is.
func TestSettlingChecksThePurchaseBelongsToTheCaller(t *testing.T) {
	src := readWalletSource(t, "stripe.go")
	if !strings.Contains(src, "belongs to another account") {
		t.Error("SettleCheckout does not check the session's account against the " +
			"caller, so pasting somebody else's session id applies their purchase")
	}
}

// Both flows return through the settling handler, not to a page that only says
// a payment worked.
func TestBothCheckoutsReturnThroughTheHandlerThatSettles(t *testing.T) {
	src := readWalletSource(t, "handlers.go")
	if n := strings.Count(src, "/wallet/stripe/success?session_id={CHECKOUT_SESSION_ID}"); n < 2 {
		t.Errorf("only %d checkout flows return with a session id — one of them lands "+
			"somewhere that cannot settle the payment", n)
	}
}

func readWalletSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
