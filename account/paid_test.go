package account

// A gift is not a signal.
//
// The whole product decides how far to trust an account on one question, and
// money is one of the three answers to it: an admin said so, a verified
// address, or money in the wallet. That third one was Balance > 0, and it was
// right for exactly as long as the only way to have a balance was to pay for
// it.
//
// New accounts are given a hundred credits now, so that somebody can ask the
// agent the question they signed up to ask. That grant made every fresh signup
// answer yes — to cold mail leaving under this instance's domain, to the
// 24-hour wait before posting, to the new-account post cap, to the agent cap,
// to the service gateway's concurrency. Six controls, all of them lifted by a
// gift the account did nothing to earn.
//
// These are the tests that would have caught it, and that fail if the two
// questions are ever made one again.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mu/internal/quota"
)

func TestTheWelcomeGrantIsNotAPayment(t *testing.T) {
	const who = "welcomed"
	if err := AddCredits(who, WelcomeCredits, OpWelcome, map[string]interface{}{
		"welcome": true,
	}); err != nil {
		t.Fatal(err)
	}

	if got := Balance(who); got != WelcomeCredits {
		t.Fatalf("balance is %d, want %d — the grant did not land and the rest of "+
			"this test would pass for the wrong reason", got, WelcomeCredits)
	}
	if Paid(who) {
		t.Error("an account holding nothing but the welcome grant reads as having paid.\n" +
			"That is the whole failure: the grant is what we gave them, and every\n" +
			"control keyed on auth.Trusted — cold mail, the 24-hour wait, the post\n" +
			"cap, the agent cap, gateway concurrency — lifts the moment it lands.")
	}
}

// The grants written before the operation had a name of its own. Those rows are
// on disk on a live instance and have to answer the same way.
func TestAnOlderWelcomeGrantIsStillNotAPayment(t *testing.T) {
	const who = "welcomed-before-the-name"
	if err := AddCredits(who, WelcomeCredits, TxTopup, map[string]interface{}{
		"welcome": true,
	}); err != nil {
		t.Fatal(err)
	}
	if Paid(who) {
		t.Error("a welcome grant written as a plain topup reads as a payment — " +
			"the metadata flag is the only handle on those and it is not being read")
	}
}

func TestPayingCounts(t *testing.T) {
	const who = "payer"
	if err := AddCredits(who, 500, quota.OpTopup, nil); err != nil {
		t.Fatal(err)
	}
	if !Paid(who) {
		t.Error("a top-up does not read as a payment, which is the signal itself")
	}

	// Ever, not now. Spending it does not un-say what it said.
	if err := DeductCredits(who, 500, "test", nil); err != nil {
		t.Fatal(err)
	}
	if got := Balance(who); got != 0 {
		t.Fatalf("balance is %d after spending it all", got)
	}
	if !Paid(who) {
		t.Error("an account that paid and then spent it reads as never having paid.\n" +
			"Trust is not a balance: the money can go back to zero, the fact cannot.")
	}
}

// Somebody else's money arriving is not this account's signal. Both of these
// are the same sybil shape — one funded account establishing a hundred empty
// ones — and the person who actually paid is already trusted on their own
// account.
func TestMoneyFromSomebodyElseIsNotAPayment(t *testing.T) {
	if err := AddCredits("funder", 1000, quota.OpTopup, nil); err != nil {
		t.Fatal(err)
	}
	if err := TransferCredits("funder", "shell", 100); err != nil {
		t.Fatal(err)
	}
	if got := Balance("shell"); got != 100 {
		t.Fatalf("the transfer did not land: balance is %d", got)
	}
	if Paid("shell") {
		t.Error("an account funded by a transfer reads as having paid — " +
			"one card would establish as many accounts as it could afford")
	}

	if err := AddCredits("seller", 1, quota.OpAppRevenue, map[string]interface{}{
		"app": "anything", "from": "funder",
	}); err != nil {
		t.Fatal(err)
	}
	if Paid("seller") {
		t.Error("app revenue reads as having paid — buying your own app for a " +
			"credit would establish the account that sold it")
	}
}

func TestAnAccountWithNothingHasNotPaid(t *testing.T) {
	if Paid("stranger") {
		t.Error("an account with no ledger at all reads as having paid")
	}
}

// Every way credits arrive is classified deliberately.
//
// Paid works by exclusion — a top-up counts unless it is named here as one of
// the ways money arrives without anybody paying for it — and exclusion fails
// open: a new credit path added tomorrow counts as a payment by default,
// silently, and nobody finds out until it is a way in.
//
// This is the ledger pattern the rest of the repo uses for the same problem:
// not a rule that forbids the edge, but a list that has to be kept honest. Add
// a way for credits to arrive and this fails until you have said which kind it
// is.
func TestEveryWayCreditsArriveIsClassified(t *testing.T) {
	// Every operation that can add credit to an account, and whether holding it
	// means the account itself paid.
	classified := map[string]bool{
		quota.OpTopup:      true,  // a card, through Stripe
		"topup_usdc":       true,  // USDC, through x402
		"admin_grant":      true,  // an operator's decision about one account
		OpWelcome:          false, // ours, handed over at signup
		quota.OpAppRevenue: false, // the buyer's, not the seller's
		quota.OpTransfer:   false, // somebody else's, moved
	}

	for op, isPayment := range classified {
		who := "classified-" + op
		if err := AddCredits(who, 10, op, nil); err != nil {
			t.Fatal(err)
		}
		if got := Paid(who); got != isPayment {
			t.Errorf("credits arriving as %q read as paid=%v, want %v", op, got, isPayment)
		}
	}

	// And the list is the whole list. Not by parsing operation names out of the
	// source — that means resolving constants and would break on a rename that
	// changed nothing — but by counting the places credit can be added at all.
	// A new one is a new way for an account to hold money, and this fails until
	// somebody has said which kind it is.
	sites := creditSites(t)
	known := map[string]string{
		"account/stripe.go":  "a card settling — quota.OpTopup",
		"account/usdc.go":    "USDC settling — topup_usdc",
		"account/credits.go": "the welcome grant, and the two ledger writes below it",
		"admin/admin.go":     "an operator granting credit — admin_grant",
	}
	for _, file := range sites {
		if _, ok := known[file]; !ok {
			t.Errorf("%s adds credit and is not accounted for above.\n"+
				"Say which operation it writes and whether an account holding it has\n"+
				"paid: Paid counts an unrecognised top-up as a payment, and a payment\n"+
				"is what lets an account mail strangers and post on its first day.", file)
		}
	}
	for file := range known {
		found := false
		for _, s := range sites {
			if s == file {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is listed as a place credit arrives and no longer adds any", file)
		}
	}
}

// creditSites names the files that can add credit to an account.
//
// Every path goes through AddCredits or CreditOnce — the two doors on the
// ledger — or writes a TxTopup row directly, which only credits.go does and
// only from inside those two.
func creditSites(t *testing.T) []string {
	t.Helper()

	root := ".."
	call := regexp.MustCompile(`\b(AddCredits|CreditOnce)\(`)

	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" || name == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		// The declarations themselves are not call sites.
		src = strings.ReplaceAll(src, "func AddCredits(", "func _(")
		src = strings.ReplaceAll(src, "func CreditOnce(", "func _(")
		if call.MatchString(src) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			seen[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("found no place credit is added — this test is reading the wrong tree")
	}
	return out
}
