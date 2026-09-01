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
// # The header keeps its chip
//
// This hid the header's balance on Home for one commit, on the rule in
// internal/app/credits.go that the same number twice on one screen reads as a
// mistake. What that produced was worse than the repetition: the header gained
// and lost an item as you moved between pages — Admin alone on Home, Admin and
// a balance everywhere else — so the one piece of chrome that is meant to be
// identical on every page was the piece that moved.
//
// The rule was about chrome, and it still holds: it was written when the number
// was in the sidebar, which is chrome, on every page at once. This is a block
// in a page's content, with a rate and two actions the chip does not have —
// the same relationship Inbox has to the Inbox in the rail. A glance and a
// place are allowed to be about the same thing.
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

	// The same shape as the two blocks above it, because they set the pattern
	// and there was no reason for a third.
	//
	// This shipped as a bare heading with a link in it and a div with no card
	// around it, sitting under two bordered blocks with plain grey headings. It
	// was the only thing in the rail that did not look like the rail. The
	// reasoning was that a card would read as pasted in — written without
	// looking at the rendered page, where both neighbours are cards.
	//
	// So: a plain heading, a bordered block, and the way to the page at the
	// foot of it. "Go to wallet" is also what makes /wallet reachable from
	// Home, which is the same job "Go to inbox" does above.
	var b strings.Builder
	b.WriteString(sectionRule("Wallet"))
	b.WriteString(`<div class="wallet-peek">`)
	for _, part := range account.BalanceBody(accountID) {
		b.WriteString(part)
	}
	b.WriteString(`<a href="/wallet" class="link">Go to wallet &rarr;</a>`)
	b.WriteString(`</div>`)
	return b.String()
}
