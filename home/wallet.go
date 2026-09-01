package home

// The balance, in the rail, at the foot.
//
// # Why it is here and not only in the header
//
// It is in the header on every page — a 17px chip with a number beside it, see
// headBalance — and that chip is a glance rather than a place. The rail on Home
// is a list of the things that are yours: what arrived, who is working, and
// what it is being paid for. The third was the only one of those with no
// standing on this screen at all, which made money the one thing you could only
// find by noticing a badge in a corner.
//
// # And why it does not appear twice
//
// internal/app/credits.go carries the reason: the balance was in the header and
// the sidebar at once for a while, and "the same number twice on one screen
// reads as a mistake". That is still true, and it is a rule about the screen
// rather than about either position — so on Home the header chip is hidden and
// this is the balance, and on every other page the chip is. One number, one
// place, always. See body.page-home #head-wallet in mu.css.
//
// # The real card, not a number
//
// This drew its own figure and its own link for one commit, and a balance
// rendered by hand is a balance that drifts from the one on /account. It is
// account.BalanceBody now — the same figure, the same "1 credit = 1¢", the same
// note to an admin that their calls are never charged, and the same Top up and
// Transfer. Those are not trimmings: the number means nothing without the rate
// beside it, and topping up is the thing somebody looking at a balance has come
// to do.
//
// # At the foot
//
// Inbox and Agents are things that want something from you. A balance does not:
// it is a standing fact, and a standing fact above two lists that need reading
// pushes what needs reading down. It goes last for the same reason it is a
// badge rather than a nav item everywhere else.

import (
	"strings"

	"mu/account"
	"mu/internal/quota"
)

// walletHTML is the balance block, or nothing at all.
//
// Nothing on an instance that does not charge — quota answers "not charging"
// where no wallet is linked in, and a balance of zero credits on a build that
// never spends them is a worry about nothing. Same condition the header chip
// uses, so the two are never in disagreement about whether money exists here.
func walletHTML(accountID string) string {
	if accountID == "" || !quota.Charging() {
		return ""
	}

	// Under a "Wallet" heading in the rail's own shape, rather than as the card
	// it is on /account: the blocks either side of it are a heading and a list,
	// and a bordered card between them reads as something that has been pasted
	// in. The card's own "Balance" title goes with the frame — two titles for
	// one number, one of them inside the other, is what nesting them would give.
	//
	// The heading is a link, and it is the only one to /wallet on this page.
	// Wallet is not in the rail — Tokens is, money is not — so the header chip
	// was the way there, and Home is the one page that hides the chip. Top up
	// and Transfer below go to their own pages and neither is the wallet
	// itself.
	var b strings.Builder
	b.WriteString(`<p class="home-section"><small><a href="/wallet">Wallet</a></small></p>`)
	b.WriteString(`<div class="home-wallet">`)
	for _, part := range account.BalanceBody(accountID) {
		b.WriteString(part)
	}
	b.WriteString(`</div>`)
	return b.String()
}
