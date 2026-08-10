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
	"mu/internal/quota"
)

// LowBalance is the balance at or below which the banner warns. A handful of
// agent queries: enough warning to top up before anything breaks, not so much
// that the banner becomes wallpaper.
const LowBalance = 20

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
	// Asked of internal/quota rather than the wallet, which this package cannot
	// see and does not need to: whether anybody is charged here, and what this
	// viewer has left. On a build with no wallet linked in, quota answers "not
	// charging" and the indicator goes quiet, which was the point of the hook
	// this replaced.
	if acc == nil || !quota.Charging() {
		return creditView{}
	}
	balance := quota.BalanceOf(acc.ID)
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
	// Suppressed while there is nothing connected — ConnectBanner is showing,
	// and "top up" is the wrong first instruction for someone who has not made
	// a call yet.
	if acc != nil && !connected(acc) {
		return ""
	}
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

// ToolCountFunc reports how many tools this instance exposes. Set by main() —
// internal/api imports this package, so this cannot import it back.
var ToolCountFunc func() int

// ConnectBanner invites someone who has not connected an agent yet to do it.
//
// This is the step the funnel was missing. The landing promises tools for
// agents; you sign up; and you arrive on a home screen of news and markets
// cards with no mention of the thing you came for. The only route onward was
// noticing "Tools" in the sidebar.
//
// Worse, the one thing that did speak to a new arrival was the credits banner,
// which says "Top up to get started" — so the first instruction anyone got
// after signing up was pay, before they had connected anything or seen a single
// call work. That is why this takes priority over that one: you cannot usefully
// spend credits until something is connected.
//
// A missing token is the signal, because a token is the credential you can
// paste. Someone who connected Claude Desktop over OAuth has no token and will
// see this too — it reads as an invitation rather than a claim about them, and
// following it tells them they are already done.
func ConnectBanner(r *http.Request) string {
	_, acc := auth.TrySession(r)
	return connectBannerFor(acc, r.URL.Path)
}

// connected reports whether an account has a credential it could hand an agent.
func connected(acc *auth.Account) bool {
	return acc != nil && len(auth.ListTokens(acc.ID)) > 0
}

func connectBannerFor(acc *auth.Account, path string) string {
	if acc == nil || connected(acc) {
		return ""
	}
	// Not on the pages that answer it.
	switch {
	case strings.HasPrefix(path, "/tools"),
		strings.HasPrefix(path, "/token"),
		strings.HasPrefix(path, "/mcp"):
		return ""
	}

	tools := ""
	if ToolCountFunc != nil {
		tools = fmt.Sprintf(" All %d of them, on one endpoint.", ToolCountFunc())
	}
	return `<div class="credits-banner connect-banner">
<strong>Connect your agent.</strong>
<span>This is the app; the tools are the other half.` + tools + `</span>
<a href="/tools">Connect →</a>
</div>`
}
