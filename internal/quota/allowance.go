package quota

// Free allowances: how much of a priced operation an account gets each day
// before the price applies.
//
// The providers underneath already work this way — Google gives ten thousand
// Routes calls a month before billing anything — and it is the right shape for
// what sits on top too. A price with no allowance is a toll booth at the front
// door: somebody trying the thing out meets a refusal on their first question,
// and the only way to avoid that was to price the operation at zero, which then
// gave away the expensive ones for ever.
//
// An allowance separates those two decisions. What a call costs us is one
// question, and how much of it a person gets for nothing is another. The first
// is arithmetic about providers; the second is a commercial choice, and it
// belongs to the operator — so both live in quota.json and neither lives in Go.
//
// Counted per account per day, in memory. Deliberately not persisted: an
// allowance is a courtesy, and a restart handing somebody a fresh one is a
// smaller problem than a disk write on every free call. What is persisted is
// the money, which is the wallet's job and always has been.

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// used counts calls per account per operation for the current day. One map so
// there is one mutex and one midnight.
var used struct {
	sync.Mutex
	day   string
	count map[string]int
}

// There was a daily allowance of credits here — a hundred a day, spent against
// any priced operation before the balance was touched — and it is gone.
//
// It existed for one reason: a new account started at zero, talking to the agent
// cost seven, so a fresh signup could do nothing at all. That is a real problem
// and this was the wrong fix for it. Charging for the agent and then handing
// back credits to cover the charge is two mechanisms cancelling out, and what
// they left behind was a number a person had to understand before they could
// ask a question.
//
// The agent is the product, not a metered capability. It is free and bounded by
// a daily count, like the outbound operations below. Credits are for what a
// third party bills us per request — a search, an image, a text — and an account
// needs none until it reaches for one of those. Nothing to grant, nothing to
// cancel out, nothing to explain.
//
// It also quietly undid a control: service/sms says in its own package comment
// that "the price does the rest of the work", and an allowance that zeroes the
// price for the first five texts takes that work away.

// today is the date the counters belong to. A string rather than a timer: the
// process may be asleep at midnight, and comparing the date is cheaper than
// arranging to be woken.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// UsedToday is how many of an operation this account has done today.
func UsedToday(account, operation string) int {
	used.Lock()
	defer used.Unlock()
	rollLocked()
	return used.count[account+"\x00"+operation]
}

// Done records that one succeeded. Called once per successful call, after the
// fact, so nothing that failed counts against an allowance or a limit.
func Done(account, operation string) {
	if account == "" {
		return
	}
	used.Lock()
	defer used.Unlock()
	rollLocked()
	used.count[account+"\x00"+operation]++
}

// LeftToday is how many more of an operation this account may do, and whether
// it is capped at all.
func LeftToday(account, operation string) (int, bool) {
	limit := LimitFor(account, operation)
	if limit == NoLimit {
		return 0, false
	}
	if left := limit - UsedToday(account, operation); left > 0 {
		return left, true
	}
	return 0, true
}

// OverLimit reports whether this account has used up its allowance of an
// operation for today, with a sentence saying so.
func OverLimit(account, operation string) (bool, string) {
	limit := LimitFor(account, operation)
	if limit == NoLimit {
		return false, ""
	}
	if limit == 0 {
		return true, "this instance has " + Describe(operation) + " turned off"
	}
	if UsedToday(account, operation) < limit {
		return false, ""
	}
	// No mention of a plan. There are none, and offering one that does not
	// exist to somebody who has just been refused is worse than the refusal.
	return true, fmt.Sprintf("that is %d today, which is this account's limit for %s — it resets at midnight",
		limit, Describe(operation))
}

// Describe is the operation in the words the price table uses, so a refusal
// reads as "your limit for Text message" rather than for "sms_send".
func Describe(operation string) string {
	if label := Label(operation); label != "" {
		return strings.ToLower(label)
	}
	return operation
}

// rollLocked throws the counters away when the date changes.
func rollLocked() {
	if d := today(); d != used.day {
		used.day, used.count = d, map[string]int{}
	}
}

// ResetAllowances forgets every count. For tests, and for an operator who has
// just changed an allowance and wants it to apply now.
func ResetAllowances() {
	used.Lock()
	defer used.Unlock()
	used.day, used.count = today(), map[string]int{}
}
