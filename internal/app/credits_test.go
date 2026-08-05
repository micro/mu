package app

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// withBalance installs a balance hook for one test.
func withBalance(t *testing.T, balance int, charging bool) {
	t.Helper()
	prev := BalanceFunc
	BalanceFunc = func(string) (int, bool) { return balance, charging }
	t.Cleanup(func() { BalanceFunc = prev })
}

// The case the whole thing exists for: a new account has nothing, and until it
// is told so the product looks broken rather than unstarted.
func TestAnEmptyBalanceSaysHowToStart(t *testing.T) {
	withBalance(t, 0, true)

	got := creditsBannerFor(&auth.Account{ID: "newcomer"}, "/home")
	if !strings.Contains(got, "Top up to get started") {
		t.Errorf("an empty balance produced %q", got)
	}
	if !strings.Contains(got, `href="/wallet"`) {
		t.Error("the banner does not say where to go")
	}
}

// Warned before it breaks, not after.
func TestALowBalanceWarnsBeforeItRunsOut(t *testing.T) {
	withBalance(t, 5, true)

	got := creditsBannerFor(&auth.Account{ID: "nearly"}, "/home")
	if !strings.Contains(got, "low on credits") {
		t.Errorf("a low balance produced %q", got)
	}
	if strings.Contains(got, "get started") {
		t.Error("a low balance was described as an empty one")
	}
}

// A funded account is not nagged.
func TestAHealthyBalanceShowsNoBanner(t *testing.T) {
	withBalance(t, LowBalance+1, true)

	if got := creditsBannerFor(&auth.Account{ID: "funded"}, "/home"); got != "" {
		t.Errorf("a funded account was shown %q", got)
	}
}

// An instance that charges nothing must never ask for money — that would be a
// lie on every self-hosted Mu.
func TestAnInstanceThatDoesNotChargeNeverAsks(t *testing.T) {
	withBalance(t, 0, false)

	if got := creditsBannerFor(&auth.Account{ID: "selfhost"}, "/home"); got != "" {
		t.Errorf("a free instance asked for a top-up: %q", got)
	}
	if got := headBalance(&auth.Account{ID: "selfhost"}); got != "" {
		t.Errorf("a free instance showed a balance: %q", got)
	}
}

// An admin is never charged. They are told that — not shown "0 credits", which
// is false, and not shown nothing, which is how the operator of an instance
// ends up unable to see whether their own payment UI works at all.
func TestAdminsSeeUnlimitedAndAreNeverAskedToTopUp(t *testing.T) {
	withBalance(t, 0, true)
	admin := &auth.Account{ID: "boss", Admin: true}

	if got := creditsBannerFor(admin, "/home"); got != "" {
		t.Errorf("an admin was asked to top up: %q", got)
	}
	head := headBalance(admin)
	if !strings.Contains(head, "∞") {
		t.Errorf("an admin's balance was %q, want the unlimited mark", head)
	}
	if strings.Contains(head, "empty") || strings.Contains(head, ">0<") {
		t.Errorf("an admin was shown an empty balance: %q", head)
	}
}

// The banner points at /wallet, so it must not appear on top of /wallet.
func TestTheBannerIsNotShownOnThePageItPointsAt(t *testing.T) {
	withBalance(t, 0, true)

	acc := &auth.Account{ID: "newcomer"}
	for _, path := range []string{"/wallet", "/wallet/add", "/wallet/transfer"} {
		if got := creditsBannerFor(acc, path); got != "" {
			t.Errorf("the banner appeared on %s", path)
		}
	}
	// …but everywhere else it does.
	if got := creditsBannerFor(acc, "/news"); got == "" {
		t.Error("the banner is missing on an ordinary page")
	}
}

// Signed out, there is no balance and nothing to top up.
func TestSignedOutSeesNothing(t *testing.T) {
	withBalance(t, 0, true)

	if got := creditsBannerFor(nil, "/home"); got != "" {
		t.Errorf("a signed-out visitor was shown %q", got)
	}
	if got := headBalance(nil); got != "" {
		t.Errorf("a signed-out visitor was shown a balance: %q", got)
	}
}

// With no wallet wired at all, every surface stays quiet.
func TestNoWalletMeansNoCreditsUI(t *testing.T) {
	prev := BalanceFunc
	BalanceFunc = nil
	t.Cleanup(func() { BalanceFunc = prev })

	acc := &auth.Account{ID: "someone"}
	if got := creditsBannerFor(acc, "/home"); got != "" {
		t.Errorf("banner without a wallet: %q", got)
	}
	if got := headBalance(acc); got != "" {
		t.Errorf("balance without a wallet: %q", got)
	}
}

// The top bar is the only place the balance appears, so it carries every state.
func TestTheHeadBalanceShowsEveryState(t *testing.T) {
	for _, c := range []struct {
		balance int
		want    string
	}{
		{0, "head-wallet empty"},
		{LowBalance, "head-wallet low"},
		{LowBalance + 1, `class="head-wallet"`},
	} {
		withBalance(t, c.balance, true)
		if got := headBalance(&auth.Account{ID: "x"}); !strings.Contains(got, c.want) {
			t.Errorf("balance %d rendered %q, want %q", c.balance, got, c.want)
		}
	}

	withBalance(t, 1200, true)
	got := headBalance(&auth.Account{ID: "x"})
	if !strings.Contains(got, ">1,200<") {
		t.Errorf("head balance rendered %q", got)
	}
	if !strings.Contains(got, `href="/wallet"`) {
		t.Error("the balance is not a link to the wallet")
	}

	// Same suppressions as everywhere else.
	withBalance(t, 0, false)
	if got := headBalance(&auth.Account{ID: "x"}); got != "" {
		t.Errorf("a free instance showed a head balance: %q", got)
	}
	withBalance(t, 0, true)
	if got := headBalance(nil); got != "" {
		t.Errorf("a signed-out visitor saw a head balance: %q", got)
	}
}

func TestFormatCredits(t *testing.T) {
	for in, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 1200: "1,200", 1234567: "1,234,567", -5: "0"} {
		if got := formatCredits(in); got != want {
			t.Errorf("formatCredits(%d) = %q, want %q", in, got, want)
		}
	}
}
