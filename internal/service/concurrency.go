package service

// How many calls an account may have running at once.
//
// This is the rate limit an MCP buyer means. The pricing page sold "higher rate
// limits" for a year against POST_LIMIT_PER_HOUR — the abuse control on writing
// to the social timeline — which is not a thing anybody wiring up an agent has
// ever wondered about. What they hit is this: an agent fans out across twenty
// tools and wants to know how many of them run at once.
//
// It is also the only axis besides the outbound sends whose cost is not already
// covered by the credits. A concurrent call is a concurrent provider call, a
// concurrent connection and concurrent memory, and eight of them arriving
// together is a different load from eight in a row, for the same eight credits.
//
// Queued rather than refused. A caller who is over its limit waits for a slot
// instead of getting an error, because the alternative asks every agent
// framework to implement backoff for a condition that lasts milliseconds — and
// a tool call that fails on a busy moment is indistinguishable, from the far
// end, from one that failed for a real reason. There is a ceiling on the wait,
// after which it is a refusal that says what happened.

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// maxWait bounds the queue. Long enough that an ordinary burst is served and
// nobody notices; short enough that a stuck slot cannot hold a caller for ever.
const maxWait = 30 * time.Second

// Concurrency is how many calls this account may have in flight, or 0 for no
// limit. Filled in by internal/server/hooks.go, from how accountable the
// account is.
//
// Nil on a build with no billing linked in — a self-hosted instance sells
// nothing, so it limits nothing.
var Concurrency func(account string) int

var (
	slotsMu sync.Mutex
	slots   = map[string]chan struct{}{}
)

// slotFor is the account's semaphore, made on first use.
func slotFor(account string, size int) chan struct{} {
	slotsMu.Lock()
	defer slotsMu.Unlock()
	c, ok := slots[account]
	if !ok || cap(c) != size {
		// A plan change resizes it. In flight calls hold the old channel and
		// release into it harmlessly; the next call gets the new size.
		c = make(chan struct{}, size)
		slots[account] = c
	}
	return c
}

// acquire takes a slot, waiting if the account is at its limit. The returned
// function gives it back and must always be called.
func acquire(ctx context.Context, account string) (func(), error) {
	if Concurrency == nil || account == "" {
		return func() {}, nil
	}
	size := Concurrency(account)
	if size <= 0 {
		return func() {}, nil
	}
	c := slotFor(account, size)

	select {
	case c <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-c }) }, nil
	default:
	}

	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case c <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-c }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("this account already has %d calls running and one has "+
			"been waiting %s for a slot — a plan raises the limit, see /plans",
			size, maxWait)
	}
}

// ResetConcurrency forgets every semaphore. For tests.
func ResetConcurrency() {
	slotsMu.Lock()
	slots = map[string]chan struct{}{}
	slotsMu.Unlock()
}
