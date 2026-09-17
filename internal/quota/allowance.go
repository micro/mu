package quota

// Daily operation caps are independent of the included credit budget.
// Buying credits never bypasses outbound messaging limits.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// used counts calls per account per operation for the current day. One map so
// there is one mutex and one midnight.
var used struct {
	sync.Mutex
	day    string
	count  map[string]int
	loaded bool
	err    error
}

// today is the date the counters belong to. A string rather than a timer: the
// process may be asleep at midnight, and comparing the date is cheaper than
// arranging to be woken.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// UsedToday is how many of an operation this account has done today.
func UsedToday(account, operation string) int {
	used.Lock()
	defer used.Unlock()
	rollLocked()
	if used.err != nil {
		return int(^uint(0) >> 1)
	}
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
	if used.err == nil {
		used.err = data.SaveJSON("quota/allowances.json", allowanceState{used.day, used.count})
	}
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

type allowanceState struct {
	Day   string
	Count map[string]int
}

// rollLocked restores the counters once, then rolls them at UTC midnight.
func rollLocked() {
	if !used.loaded {
		used.loaded = true
		var saved allowanceState
		err := data.LoadJSON("quota/allowances.json", &saved)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			used.err = err
		}
		if err == nil {
			used.day, used.count = saved.Day, saved.Count
		}
	}
	if used.count == nil {
		used.count = map[string]int{}
	}

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
	used.loaded = true
	used.err = data.SaveJSON("quota/allowances.json", allowanceState{used.day, used.count})
}
