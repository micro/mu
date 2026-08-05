package app

// Telling someone what they have before they need it.
//
// A first-time user could sign in, find the tools, mint a token for their
// agent, and get as far as a call failing before anything on the site had
// mentioned that calls cost credits and that a new account has none. The
// balance existed only on /wallet, which is a page you visit because you
// already know you have a problem.
//
// Three things fix that, and they are the three every metered product does:
// the balance is always in view, an empty one says so before you spend, and
// the refusal at the moment of failure says what to do about it rather than
// only what went wrong.
//
// None of it applies on an instance with payments switched off — a
// self-hosted Mu charges nothing, and a banner asking for money there would
// be a lie.

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/auth"
)

// BalanceFunc reports an account's credit balance and whether this instance
// charges at all. Set by main() to the wallet's own lookup.
//
// A hook rather than an import because wallet imports this package — the same
// reason EmailSender and LinkCodeFunc are hooks. Nil means no wallet is wired,
// and everything here goes quiet.
var BalanceFunc func(accountID string) (balance int, charging bool)

// LowBalance is the balance at or below which the banner warns. A handful of
// agent queries: enough warning to top up before anything breaks, not so much
// that the banner becomes wallpaper.
const LowBalance = 20

// Balance returns the viewer's balance and whether this instance charges.
func Balance(r *http.Request) (int, bool) {
	if BalanceFunc == nil {
		return 0, false
	}
	_, acc := auth.TrySession(r)
	if acc == nil {
		return 0, false
	}
	// Admins are never charged, so a balance beside their name would be
	// meaningless and a top-up banner actively wrong.
	if acc.Admin {
		return 0, false
	}
	return BalanceFunc(acc.ID)
}

// creditView is what the viewer should be told about their credits.
type creditView struct {
	Show      bool   // render an indicator at all
	Unlimited bool   // this viewer is never charged
	Balance   int    // meaningless when Unlimited
	State     string // "", " low", " empty" — the class that colours it
}

// creditsFor decides what to show. Empty and low are coloured because a balance
// rendered like every other number is a number nobody reads.
//
// Admins are never charged, so they are shown "Unlimited" rather than nothing.
// Hiding it was worse than it sounds: the operator of an instance is an admin,
// so the one person who needs to check that the payment UI works was the one
// person who could not see it, and an empty top bar reads as a broken deploy.
// A balance of "0" would be a lie, so the indicator says the true thing.
func creditsFor(acc *auth.Account) creditView {
	if BalanceFunc == nil || acc == nil {
		return creditView{}
	}
	balance, charging := BalanceFunc(acc.ID)
	if !charging {
		return creditView{}
	}
	if acc.Admin {
		return creditView{Show: true, Unlimited: true}
	}
	switch {
	case balance <= 0:
		return creditView{Show: true, Balance: balance, State: " empty"}
	case balance <= LowBalance:
		return creditView{Show: true, Balance: balance, State: " low"}
	}
	return creditView{Show: true, Balance: balance}
}

// headBalance renders the balance in the top bar, beside Tools.
//
// One place, not two. It was briefly in the sidebar as well — top bar on a
// phone, sidebar on a desktop — and seeing the same number twice on one screen
// reads as a mistake rather than as thoroughness. The top bar is the one spot
// that is fixed, always visible, and identical on every viewport, so it is the
// spot that keeps it.
func headBalance(acc *auth.Account) string {
	v := creditsFor(acc)
	if !v.Show {
		return ""
	}
	badge := formatCredits(v.Balance)
	if v.Unlimited {
		badge = "∞"
	}
	return fmt.Sprintf(`<a id="head-wallet" class="head-wallet%s" href="/wallet" aria-label="Credits">`+
		`<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" `+
		`stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`+
		`<path d="M16 12h.01"/></svg><span id="head-wallet-badge">%s</span></a>`,
		v.State, badge)
}

// formatCredits groups thousands, so 1200 reads as 1,200.
func formatCredits(n int) string {
	if n < 0 {
		n = 0
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

// CreditsBanner invites a top-up when the viewer cannot pay for much.
//
// Empty is the case that matters: a new account has no credits, and until it
// has some the agent and every paid tool will refuse. Saying so on arrival is
// the difference between a product that looks broken and one that looks
// unstarted.
func CreditsBanner(r *http.Request) string {
	_, acc := auth.TrySession(r)
	return creditsBannerFor(acc, r.URL.Path)
}

// creditsBannerFor is the banner itself. It takes an account rather than a
// request because the home screen renders from an account with no request in
// hand — and the home screen is the first page a new user sees, so it is the
// one place the banner must not be missing.
//
// path may be empty when the caller does not know it; it only suppresses the
// banner on the page it would point at.
func creditsBannerFor(acc *auth.Account, path string) string {
	v := creditsFor(acc)
	// Unlimited cannot run out, so a top-up prompt would be a lie even though
	// the indicator is shown.
	if !v.Show || v.Unlimited || v.Balance > LowBalance {
		return ""
	}
	balance := v.Balance
	// Not on the wallet pages: the top-up options are already there, and a
	// banner above them pointing at them reads as a fault.
	if strings.HasPrefix(path, "/wallet") {
		return ""
	}

	headline, detail := "You're low on credits.", fmt.Sprintf(
		"%s left. Paid tools and the agent stop working at zero.", formatCredits(balance))
	if balance <= 0 {
		headline = "Top up to get started."
		detail = "Your balance is empty. The agent and paid tools need credits to run — " +
			"reading, listing and everything this instance stores stays free."
	}

	return `<div class="credits-banner">
<strong>` + headline + `</strong>
<span>` + detail + `</span>
<a href="/wallet">Top up →</a>
</div>`
}
