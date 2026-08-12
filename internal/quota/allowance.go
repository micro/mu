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

// used counts calls per account per operation for the current day.
var used struct {
	sync.Mutex
	day   string
	count map[string]int
}

// today is the date the counters belong to. A string rather than a timer: the
// process may be asleep at midnight, and comparing the date is cheaper than
// arranging to be woken.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// FreeCallsLeft is how many more of this operation the account may make today
// before it starts costing credits.
func FreeCallsLeft(account, operation string) int {
	allowance := FreeAllowance(operation)
	if allowance <= 0 {
		return 0
	}
	used.Lock()
	defer used.Unlock()
	rollLocked()
	if n := used.count[account+"\x00"+operation]; n < allowance {
		return allowance - n
	}
	return 0
}

// WithinAllowance reports whether this call is free because the account has not
// used up its allowance today.
//
// It used to count as well as ask, on the argument that a caller who asked and
// then forgot to say it happened would hand out the same allowance for ever.
// That was true and the fix was in the wrong place: it counted before the call
// ran, so a call that failed still spent an allowance, and it meant one counter
// was incremented from here while the limits below needed it incremented after
// success. One counter, moved once, by Done.
func WithinAllowance(account, operation string) bool {
	allowance := FreeAllowance(operation)
	if allowance <= 0 || account == "" {
		return false
	}
	used.Lock()
	defer used.Unlock()
	rollLocked()
	return used.count[account+"\x00"+operation] < allowance
}

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
	return true, fmt.Sprintf("that is %d today, which is this account's limit for %s — it resets at midnight, and a plan raises it",
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
