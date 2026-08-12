package service

// One request, one charge — the test for the 60-credit image.
//
// A live image generation debited four charges of 15 and produced one image.
// The endpoint polls a provider for up to 150 seconds, which is longer than a
// lot of the read timeouts between a caller and here, so what arrives is one
// request several times over from software doing the correct thing with a
// response it never saw.
//
// These are the two arrangements that has to survive: overlapping identical
// calls are one call, and sequential identical calls are two.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// held blocks the slow handler until a test releases it, so two calls can be
// made to overlap deliberately rather than by racing them and hoping.
var held = struct {
	sync.Mutex
	ch   chan struct{}
	runs int
}{}

type SlowReq struct {
	Prompt string `json:"prompt"`
}

// Slow stands in for image generation: it takes long enough that a caller gives
// up on it, and it counts how many times it actually ran.
func (Tolled) Slow(_ context.Context, req *SlowReq, rsp *TollRsp) error {
	held.Lock()
	ch := held.ch
	held.runs++
	held.Unlock()
	if ch != nil {
		<-ch
	}
	rsp.Text = "generated " + req.Prompt
	return nil
}

func holdSlow(t *testing.T) (release func()) {
	t.Helper()
	held.Lock()
	held.ch = make(chan struct{})
	held.runs = 0
	ch := held.ch
	held.Unlock()
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(ch) }) })
	return func() { once.Do(func() { close(ch) }) }
}

func slowRuns() int {
	held.Lock()
	defer held.Unlock()
	return held.runs
}

func registerSlow(t *testing.T, name string) {
	t.Helper()
	if _, already := SpecFor(name); already {
		return
	}
	err := Register(Spec{
		Name:    name,
		Handler: new(Tolled),
		Endpoints: map[string]Endpoint{
			"Slow":   {Doc: "costs something and takes a while", Cost: "slow_op"},
			"Priced": {Doc: "costs something", Cost: "test_op"},
			"Free":   {Doc: "costs nothing"},
		},
	})
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func callSlow(ctx context.Context, name string, req *SlowReq) (string, error) {
	var rsp TollRsp
	err := Call(ctx, name, "Tolled.Slow", req, &rsp)
	return rsp.Text, err
}

// TestARetryOfACallStillRunningIsNotASecondCall — the defect, directly.
//
// Four arrivals of one image generation. The handler runs once, the provider is
// paid once, the account is charged once, and all four callers get the image.
func TestARetryOfACallStillRunningIsNotASecondCall(t *testing.T) {
	const svc = "dedupe-inflight"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	release := holdSlow(t)

	ctx := WithAccount(context.Background(), "asim")
	req := &SlowReq{Prompt: "a red square"}

	var wg sync.WaitGroup
	answers := make([]string, 4)
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			answers[i], errs[i] = callSlow(ctx, svc, req)
		}(i)
	}

	// Let all four arrive and pile up on the one that is running.
	waitFor(t, func() bool { return slowRuns() > 0 })
	time.Sleep(20 * time.Millisecond)
	release()
	wg.Wait()

	if n := slowRuns(); n != 1 {
		t.Errorf("the handler ran %d times for one request — the provider was paid %d times", n, n)
	}
	allowed, charged := l.counts()
	if charged != 1 {
		t.Errorf("one request was charged %d times (asked %d) — this is the 60-credit image",
			charged, allowed)
	}
	for i := range answers {
		if errs[i] != nil {
			t.Errorf("caller %d got an error: %v", i, errs[i])
		}
		if answers[i] != "generated a red square" {
			t.Errorf("caller %d got %q — a duplicate must still get the answer, or it "+
				"retries again and the caller is worse off than before", i, answers[i])
		}
	}
}

// TestAskingTwiceIsTwoRequests — the other half, and the reason this is not a
// response cache. Somebody who wants two images of a red square asks twice, in
// a row, having seen the first. Both are made and both are charged.
func TestAskingTwiceIsTwoRequests(t *testing.T) {
	const svc = "dedupe-sequential"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	holdSlow(t)()

	ctx := WithAccount(context.Background(), "asim")
	req := &SlowReq{Prompt: "a red square"}
	for i := 0; i < 2; i++ {
		if _, err := callSlow(ctx, svc, req); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := slowRuns(); n != 2 {
		t.Errorf("two separate requests ran the handler %d times, want 2", n)
	}
	if _, charged := l.counts(); charged != 2 {
		t.Errorf("two separate requests were charged %d times, want 2", charged)
	}
}

// TestTwoAccountsAreNeverTheSameRequest — the account is in the key, and it has
// to be: collapsing these would charge one of them nothing and hand it the
// other's answer.
func TestTwoAccountsAreNeverTheSameRequest(t *testing.T) {
	const svc = "dedupe-accounts"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	release := holdSlow(t)

	req := &SlowReq{Prompt: "a red square"}
	var wg sync.WaitGroup
	for _, who := range []string{"asim", "someone-else"} {
		wg.Add(1)
		go func(who string) {
			defer wg.Done()
			callSlow(WithAccount(context.Background(), who), svc, req) //nolint:errcheck
		}(who)
	}
	waitFor(t, func() bool { return slowRuns() == 2 })
	release()
	wg.Wait()

	if _, charged := l.counts(); charged != 2 {
		t.Errorf("two accounts asking the same thing were charged %d times, want 2", charged)
	}
}

// TestARequestIdIsHonouredAfterTheCallHasFinished — an explicit id is the
// caller saying "this is the same request" in as many words, so it survives the
// call completing. Without one a repeat is a repeat.
func TestARequestIdIsHonouredAfterTheCallHasFinished(t *testing.T) {
	const svc = "dedupe-explicit"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	holdSlow(t)()

	ctx := WithRequest(WithAccount(context.Background(), "asim"), "req-1")
	first, err := callSlow(ctx, svc, &SlowReq{Prompt: "a red square"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	// The retry arrives after the first finished, and with different arguments —
	// what makes it the same request is the id, not the body.
	second, err := callSlow(ctx, svc, &SlowReq{Prompt: "a blue square"})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if n := slowRuns(); n != 1 {
		t.Errorf("a retry under the same request id ran the handler %d times, want 1", n)
	}
	if _, charged := l.counts(); charged != 1 {
		t.Errorf("a retry under the same request id was charged %d times, want 1", charged)
	}
	if second != first {
		t.Errorf("the retry got %q and the first call got %q — a replay has to be the "+
			"same answer, or the id promised something it did not keep", second, first)
	}
}

// TestADifferentRequestIdIsADifferentRequest.
func TestADifferentRequestIdIsADifferentRequest(t *testing.T) {
	const svc = "dedupe-two-ids"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	holdSlow(t)()

	base := WithAccount(context.Background(), "asim")
	for _, id := range []string{"req-1", "req-2"} {
		if _, err := callSlow(WithRequest(base, id), svc, &SlowReq{Prompt: "a red square"}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if _, charged := l.counts(); charged != 2 {
		t.Errorf("two request ids were charged %d times, want 2", charged)
	}
}

// TestAFailureIsNotReplayed — the caller retried because it got no answer.
// Handing back the first attempt's error would turn one transient provider
// failure into a permanent one for as long as the entry lived.
func TestAFailureIsNotReplayed(t *testing.T) {
	const svc = "dedupe-failure"
	registerTolled(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)

	ctx := WithRequest(WithAccount(context.Background(), "asim"), "req-flaky")
	if err := callTolled(ctx, svc, "Priced", &TollReq{Fail: true}); err == nil {
		t.Fatal("the handler was supposed to fail")
	}
	if err := callTolled(ctx, svc, "Priced", &TollReq{}); err != nil {
		t.Fatalf("the retry of a failed call was refused with the first one's error: %v", err)
	}
	if _, charged := l.counts(); charged != 1 {
		t.Errorf("charged %d times — the failure is free and the retry that worked is not", charged)
	}
}

// TestAFreeCallIsNotDeduplicated — nothing is saved by collapsing a call that
// costs nobody anything, and collapsing it would be the gateway deciding it
// knows better than a caller who asked twice.
func TestAFreeCallIsNotDeduplicated(t *testing.T) {
	const svc = "dedupe-free"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	release := holdSlow(t)

	ctx := WithAccount(context.Background(), "asim")
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var rsp TollRsp
			Call(ctx, svc, "Tolled.Free", &TollReq{}, &rsp) //nolint:errcheck
		}()
	}
	wg.Wait()
	release()
	if allowed, charged := l.counts(); allowed != 0 || charged != 0 {
		t.Errorf("a free call asked %d and charged %d", allowed, charged)
	}
}

// TestFinishedCallsAreForgotten — the table is requests in the last few
// minutes, not a cache that grows for the life of the process.
func TestFinishedCallsAreForgotten(t *testing.T) {
	const svc = "dedupe-forget"
	registerSlow(t, svc)
	ResetDedupe()
	l := &ledger{}
	l.install(t)
	holdSlow(t)()

	ctx := WithAccount(context.Background(), "asim")
	for i := 0; i < 20; i++ {
		if _, err := callSlow(ctx, svc, &SlowReq{Prompt: fmt.Sprintf("square %d", i)}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	callsMu.Lock()
	n := len(calls)
	callsMu.Unlock()
	if n != 0 {
		t.Errorf("%d finished calls with no request id are still remembered — without an "+
			"id there is nothing to replay them for, so they are pure growth", n)
	}
}

// waitFor spins until cond, or fails the test. Used instead of a fixed sleep so
// a slow machine does not turn a passing test into a flake.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the calls to arrive")
		}
		time.Sleep(time.Millisecond)
	}
}
