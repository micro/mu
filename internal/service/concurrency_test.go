package service

// The limit an agent actually meets.
//
// A card that says "8 tool calls at once" has to mean it, or it is the same
// defect as "higher rate limits": a number sold against nothing.

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestCallsBeyondTheLimitWaitRatherThanFail.
func TestCallsBeyondTheLimitWaitRatherThanFail(t *testing.T) {
	ResetConcurrency()
	orig := Concurrency
	t.Cleanup(func() { Concurrency = orig; ResetConcurrency() })
	Concurrency = func(string) int { return 2 }

	a, err := acquire(context.Background(), "acct")
	if err != nil {
		t.Fatalf("first call refused: %v", err)
	}
	b, err := acquire(context.Background(), "acct")
	if err != nil {
		t.Fatalf("second call refused: %v", err)
	}

	// The third has nowhere to go until one of the first two finishes.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan error, 1)
	go func() {
		rel, err := acquire(ctx, "acct")
		if rel != nil {
			rel()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("a third call went straight through with a limit of 2 (err=%v)", err)
	case <-time.After(50 * time.Millisecond):
	}

	a()
	select {
	case err := <-got:
		if err != nil {
			t.Errorf("the waiting call failed once a slot freed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("a slot was released and the waiting call never took it")
	}
	b()
}

// TestTwoAccountsDoNotQueueBehindEachOther — the limit is per account, or one
// busy customer is every other customer's problem.
func TestTwoAccountsDoNotQueueBehindEachOther(t *testing.T) {
	ResetConcurrency()
	orig := Concurrency
	t.Cleanup(func() { Concurrency = orig; ResetConcurrency() })
	Concurrency = func(string) int { return 1 }

	busy, err := acquire(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	defer busy()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		rel, err := acquire(ctx, "two")
		if rel != nil {
			rel()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a second account was refused while the first was busy: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("a second account queued behind the first, so one caller can stall everybody")
	}
	cancel()
}

// TestNoLimitWhenNothingIsSold — a self-hosted instance queues nobody.
func TestNoLimitWhenNothingIsSold(t *testing.T) {
	ResetConcurrency()
	orig := Concurrency
	t.Cleanup(func() { Concurrency = orig })
	Concurrency = nil

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := acquire(context.Background(), "acct")
			if err != nil {
				t.Errorf("an unmetered instance queued a call: %v", err)
				return
			}
			rel()
		}()
	}
	wg.Wait()
}

// TestASlotIsReleasedOnce — release is deferred and may be called again by a
// retry path; giving a slot back twice would let an account exceed its limit
// for ever.
func TestASlotIsReleasedOnce(t *testing.T) {
	ResetConcurrency()
	orig := Concurrency
	t.Cleanup(func() { Concurrency = orig; ResetConcurrency() })
	Concurrency = func(string) int { return 1 }

	rel, err := acquire(context.Background(), "acct")
	if err != nil {
		t.Fatal(err)
	}
	rel()
	rel()

	// One slot, so a second acquire must succeed and a third must wait.
	first, err := acquire(context.Background(), "acct")
	if err != nil {
		t.Fatal(err)
	}
	defer first()

	// Cancelled and waited for, so the goroutine cannot still be reading the
	// Concurrency hook while this test's cleanup puts the old one back.
	ctx, cancel := context.WithCancel(context.Background())
	blocked := make(chan struct{})
	go func() {
		defer close(blocked)
		rel, _ := acquire(ctx, "acct")
		if rel != nil {
			rel()
		}
	}()
	select {
	case <-blocked:
		t.Error("a double release widened the limit — two calls run where one is allowed")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	<-blocked
}
