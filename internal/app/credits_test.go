package app

import (
	"strings"
	"testing"
	"time"

	"mu/internal/quota"

	"mu/internal/auth"
)

// withToken returns an account that has connected something, because the
// credits banner stays quiet until then — "top up" is the wrong first
// instruction for someone who has not made a call yet.
func withToken(t *testing.T, id string) *auth.Account {
	t.Helper()
	acc := &auth.Account{ID: id, Name: id, Secret: "test-secret"}
	auth.Create(acc)
	// Not an admin: they are never charged, so they are shown no balance and no
	// top-up banner. Stated here rather than assumed, because auth.Create makes
	// the first account on an empty instance an admin and these tests are about
	// somebody ordinary.
	acc.Admin = false
	if _, _, err := auth.CreateToken(id, "test", nil, time.Time{}); err != nil {
		t.Fatalf("create token: %v", err)
	}
	return acc
}

// withBalance installs a balance hook for one test.
func withBalance(t *testing.T, balance int, charging bool) {
	t.Helper()
	prevEnabled, prevBalance := quota.Enabled, quota.Balance
	quota.Enabled = func() bool { return charging }
	quota.Balance = func(string) int { return balance }
	t.Cleanup(func() { quota.Enabled, quota.Balance = prevEnabled, prevBalance })
}

// The case the whole thing exists for: a new account has nothing, and until it
// is told so the product looks broken rather than unstarted.
func TestAnEmptyBalanceSaysHowToStart(t *testing.T) {
	withBalance(t, 0, true)

	got := creditsBannerFor(withToken(t, "newcomer"), "/home")
	if !strings.Contains(got, "Top up to get started") {
		t.Errorf("an empty balance produced %q", got)
	}
	if !strings.Contains(got, `href="/wallet/topup"`) {
		t.Error("the banner does not say where to go")
	}
}

// Warned before it breaks, not after.
func TestALowBalanceWarnsBeforeItRunsOut(t *testing.T) {
	withBalance(t, 5, true)

	got := creditsBannerFor(withToken(t, "nearly"), "/home")
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

	if got := creditsBannerFor(withToken(t, "funded"), "/home"); got != "" {
		t.Errorf("a funded account was shown %q", got)
	}
}

// An instance that charges nothing must never ask for money — that would be a
// lie on every self-hosted Mu.
func TestAnInstanceThatDoesNotChargeNeverAsks(t *testing.T) {
	withBalance(t, 0, false)

	if got := creditsBannerFor(withToken(t, "selfhost"), "/home"); got != "" {
		t.Errorf("a free instance asked for a top-up: %q", got)
	}
	if got := headBalance(&auth.Account{ID: "selfhost"}); got != "" {
		t.Errorf("a free instance showed a balance: %q", got)
	}
}

// An admin is never charged, so there is no number and nothing to warn about.
//
// The corner said "∞" for them, on the argument that showing nothing is how the
// operator of an instance ends up unable to see whether their own payment UI
// works — an empty top bar reads as a broken deploy. That was true when the
// corner held only this and Admin, both conditional, so an admin on an
// unmetered instance got a genuinely empty corner. It is not true now: the
// corner carries @you on every page, so there is no blank to misread, and a
// permanent ∞ is a fact about billing shown to the one person who cannot be
// billed, on every page they will ever look at.
//
// What has to keep holding is the half that was never about display: an admin
// is not asked to top up.
func TestAdminsAreNeverAskedToTopUp(t *testing.T) {
	withBalance(t, 0, true)
	admin := withToken(t, "boss")
	admin.Admin = true

	if got := creditsBannerFor(admin, "/home"); got != "" {
		t.Errorf("an admin was asked to top up: %q", got)
	}
	head := headBalance(admin)
	if strings.Contains(head, "empty") || strings.Contains(head, ">0<") {
		t.Errorf("an admin was shown an empty balance: %q", head)
	}
	if head != "" {
		t.Errorf("an admin carries a balance indicator on every page: %q", head)
	}
}

// The banner points at the account, so it must not appear on top of it.
func TestTheBannerIsNotShownOnThePageItPointsAt(t *testing.T) {
	withBalance(t, 0, true)

	acc := withToken(t, "newcomer2")
	for _, path := range []string{"/account", "/wallet/topup", "/wallet/transfer"} {
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
	prev := quota.Enabled
	quota.Enabled = nil
	t.Cleanup(func() { quota.Enabled = prev })

	acc := &auth.Account{ID: "someone"}
	if got := creditsBannerFor(acc, "/home"); got != "" {
		t.Errorf("banner without a wallet: %q", got)
	}
	if got := headBalance(acc); got != "" {
		t.Errorf("balance without a wallet: %q", got)
	}
}

// The corner speaks when running out, and not otherwise.
//
// It showed the balance always, which is a meter for something nobody is
// thinking about: an assistant that keeps your remaining credit in the header
// of every page is asking you to watch a number instead of asking it things.
// What is worth interrupting for is running out — and empty and low were
// already the only two states the stylesheet bothered to colour, which is this
// same judgement made once and then not acted on.
//
// The number is on /wallet and /account for anybody who wants it, and the agent
// says what a refusal cost.
func TestTheHeadBalanceSpeaksOnlyWhenItMatters(t *testing.T) {
	for _, c := range []struct {
		balance int
		want    string
	}{
		{0, "head-wallet empty"},
		{LowBalance, "head-wallet low"},
	} {
		withBalance(t, c.balance, true)
		if got := headBalance(&auth.Account{ID: "x"}); !strings.Contains(got, c.want) {
			t.Errorf("balance %d rendered %q, want %q", c.balance, got, c.want)
		}
	}
	// And says nothing at a balance nobody needs to act on.
	withBalance(t, LowBalance+1, true)
	if got := headBalance(&auth.Account{ID: "x"}); got != "" {
		t.Errorf("a healthy balance is in the corner of every page: %q", got)
	}
	withBalance(t, 1200, true)
	if got := headBalance(&auth.Account{ID: "x"}); got != "" {
		t.Errorf("a healthy balance is in the corner of every page: %q", got)
	}

	// When it does speak, it leads where the money is.
	withBalance(t, 0, true)
	if got := headBalance(&auth.Account{ID: "x"}); !strings.Contains(got, `href="/wallet"`) {
		t.Errorf("the warning is not a link to where the balance lives: %q", got)
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

// Connect comes before top up. The landing promises tools for agents; you sign
// up; and the first thing the product used to say was "Top up to get started",
// before you had connected anything or seen a call work.
func TestConnectingComesBeforeToppingUp(t *testing.T) {
	withBalance(t, 0, true)

	fresh := &auth.Account{ID: "unconnected", Name: "unconnected", Secret: "s"}
	auth.Create(fresh)

	if got := creditsBannerFor(fresh, "/home"); got != "" {
		t.Errorf("someone with nothing connected was asked to top up: %q", got)
	}

	// …and once something is connected, the credits banner has the floor again.
	if got := creditsBannerFor(withToken(t, "nowconnected"), "/home"); got == "" {
		t.Error("a connected account with no credits was told nothing")
	}
}

// The invitation is for someone who has not connected anything, and it does not
// appear on the pages that answer it.
func TestConnectBannerInvitesAndThenGetsOutOfTheWay(t *testing.T) {
	prev := ToolCountFunc
	ToolCountFunc = func() int { return 72 }
	t.Cleanup(func() { ToolCountFunc = prev })

	fresh := &auth.Account{ID: "invitee", Name: "invitee", Secret: "s"}
	auth.Create(fresh)

	got := connectBannerFor(fresh, "/home")
	if !strings.Contains(got, "Connect your agent") || !strings.Contains(got, `href="/tools"`) {
		t.Errorf("no invitation on the home screen: %q", got)
	}
	if !strings.Contains(got, "All 72 of them") {
		t.Errorf("the tool count is not counted: %q", got)
	}

	// Not on the pages that answer it.
	for _, path := range []string{"/tools", "/token", "/mcp"} {
		if b := connectBannerFor(fresh, path); b != "" {
			t.Errorf("the invitation appeared on %s", path)
		}
	}

	// Gone once there is a token to paste.
	if _, _, err := auth.CreateToken(fresh.ID, "t", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if b := connectBannerFor(fresh, "/home"); b != "" {
		t.Errorf("still inviting after connecting: %q", b)
	}

	// And never to a signed-out visitor.
	if b := connectBannerFor(nil, "/home"); b != "" {
		t.Errorf("a signed-out visitor was invited: %q", b)
	}
}
