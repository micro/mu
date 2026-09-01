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
// # At the foot
//
// Inbox and Agents are things that want something from you. A balance does not:
// it is a standing fact, and a standing fact above two lists that need reading
// pushes what needs reading down. It goes last for the same reason it is a
// badge rather than a nav item everywhere else.

import (
	"html"
	"strconv"
	"strings"

	"mu/internal/auth"
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

	// Unlimited for an admin, which is what the header shows them too. Reading
	// the account rather than trusting the caller: this draws money, and the
	// session is the only thing that says whose.
	balance := ""
	if acc, err := auth.GetAccount(accountID); err == nil && acc != nil && acc.Admin {
		balance = "∞"
	} else {
		balance = groupThousands(quota.BalanceOf(accountID))
	}

	var b strings.Builder
	b.WriteString(sectionRule("Wallet"))
	b.WriteString(`<a class="home-wallet" href="/wallet">`)
	b.WriteString(`<span class="home-wallet-n">` + html.EscapeString(balance) + `</span>`)
	// The word, because a number on its own in a rail of lists is a number
	// without a unit — and "credits" is what the wallet page, the pricing and
	// the refusals all call it.
	b.WriteString(`<span class="home-wallet-w">credits</span>`)
	b.WriteString(`</a>`)
	return b.String()
}

// groupThousands is 1200 as 1,200, matching the header's badge.
//
// Duplicated rather than exported from internal/app, which formats it for a
// badge that is its own: one unexported six-line helper in two packages is a
// smaller thing than a formatting function in the app package's API, and the
// two would not be allowed to diverge anyway — see the test.
func groupThousands(n int) string {
	if n < 0 {
		n = 0
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
