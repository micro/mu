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
// used up its allowance today, and counts it if so.
//
// Counting here rather than in a separate step is what keeps the two in step: a
// caller that asked whether a call was free and then forgot to say it had
// happened would hand out the same allowance for ever.
func WithinAllowance(account, operation string) bool {
	allowance := FreeAllowance(operation)
	if allowance <= 0 || account == "" {
		return false
	}
	used.Lock()
	defer used.Unlock()
	rollLocked()
	key := account + "\x00" + operation
	if used.count[key] >= allowance {
		return false
	}
	used.count[key]++
	return true
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
