package account

// A payment must land whether or not the webhook ever arrives.
//
// Everything hung on one: the card was charged, Stripe called us, and that call
// was the only thing that credited an account or recorded a plan. If it was not
// configured, or its secret was wrong, or its event list was missing the one
// that mattered, the money moved and nothing here did — no credits, no plan, no
// error, nothing in the log to notice. And it is not a subscription problem: a
// one-off top-up hung on exactly the same thread.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/data"
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
	before := Balance(owner)

	settleSession(checkoutSession{
		ID: "cs_settle_1", PaymentStatus: "paid", Customer: "cus_x",
		AmountTotal: 2000, UserID: owner, Credits: "2000",
	}, "test")

	if got := Balance(owner); got != before+2000 {
		t.Errorf("balance is %d, want %d", got, before+2000)
	}
	acc, _ := auth.GetAccount(owner)
	if acc.Customer != "cus_x" {
		t.Errorf("customer is %q, want cus_x", acc.Customer)
	}
}

// The whole point of one settle function: two routes, one effect.
func TestTheSameSessionSettlesOnlyOnce(t *testing.T) {
	const owner = "settle-twice"
	settler(t, owner)
	before := Balance(owner)

	s := checkoutSession{
		ID: "cs_settle_2", PaymentStatus: "paid",
		AmountTotal: 500, UserID: owner, Credits: "500",
	}
	settleSession(s, "webhook")
	settleSession(s, "return from checkout")

	if got := Balance(owner); got != before+500 {
		t.Errorf("balance is %d, want %d — the session was applied twice, which is "+
			"what a second settling route would cost if it were not deduped",
			got, before+500)
	}
}

// And it survives a restart, which is the case the old guard could not.
//
// Settling was deduped against a map of seen session ids held in memory. Stripe
// retries a delivery for up to three days, so the sequence that double-credited
// somebody was ordinary: charge, settle, deploy, retry. Nothing about it needed
// bad luck — a deploy inside three days of a top-up is most deploys.
//
// There is no map to clear here because there is no map: the guard is the
// ledger, and the ledger is what a restart reloads. Wiping every scrap of
// in-process state that is not the ledger is exactly what a restart does, so
// that is what this does.
func TestASettledSessionStaysSettledAcrossARestart(t *testing.T) {
	const owner = "settle-restart"
	settler(t, owner)
	before := Balance(owner)

	s := checkoutSession{
		ID: "cs_settle_restart", PaymentStatus: "paid",
		AmountTotal: 700, UserID: owner, Credits: "700",
	}
	settleSession(s, "webhook")
	if got := Balance(owner); got != before+700 {
		t.Fatalf("the first settle credited %d, want %d", got-before, 700)
	}

	// The restart: drop the loaded balances and read them back from disk, the
	// way init does. Anything remembering this payment other than the ledger
	// does not come back.
	mutex.Lock()
	balances = map[string]*Credits{}
	b, _ := data.LoadFile("wallets.json")
	json.Unmarshal(b, &balances) //nolint:errcheck
	mutex.Unlock()

	settleSession(s, "webhook retry")

	if got := Balance(owner); got != before+700 {
		t.Errorf("balance is %d, want %d — a retried delivery credited the same "+
			"payment twice after a restart", got, before+700)
	}
}

// The other half: a genuinely different payment is not swallowed by the dedupe.
func TestADifferentSessionStillCredits(t *testing.T) {
	const owner = "settle-distinct"
	settler(t, owner)
	before := Balance(owner)

	for _, id := range []string{"cs_distinct_a", "cs_distinct_b"} {
		settleSession(checkoutSession{
			ID: id, PaymentStatus: "paid",
			AmountTotal: 300, UserID: owner, Credits: "300",
		}, "webhook")
	}

	if got := Balance(owner); got != before+600 {
		t.Errorf("balance is %d, want %d — the dedupe key is not the session", got, before+600)
	}
}

// Nothing lands until Stripe says it was paid.
func TestAnUnpaidSessionChangesNothing(t *testing.T) {
	const owner = "settle-unpaid"
	settler(t, owner)
	before := Balance(owner)

	settleSession(checkoutSession{
		ID: "cs_settle_3", PaymentStatus: "unpaid",
		UserID: owner, Credits: "5000",
	}, "test")

	if got := Balance(owner); got != before {
		t.Errorf("an unpaid session credited %d", got-before)
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

// The checkout returns through the handler that settles, not to a page that
// only says a payment worked.
func TestCheckoutReturnsThroughTheHandlerThatSettles(t *testing.T) {
	src := readWalletSource(t, "balance.go")
	if !strings.Contains(src, "/account/stripe/success?session_id={CHECKOUT_SESSION_ID}") {
		t.Error("the checkout does not return with a session id, so it lands " +
			"somewhere that cannot settle the payment if the webhook never arrives")
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
