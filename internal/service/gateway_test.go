package service

// The gateway is only worth anything if every call really goes through it.
//
// This is the test that would have caught the thing it was built for: prices
// declared on Specs that nothing ever debited. web_search, weather_forecast and
// every routes endpoint carried a Cost and were free in practice, because the
// charge lived in whichever of four places the service had been written
// against — or in none of them.

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// tolled is a service with one priced method and one free one.
type Tolled struct{}

type TollReq struct {
	Fail bool `json:"fail"`
}
type TollRsp struct {
	Text string `json:"text"`
}

func (Tolled) Priced(_ context.Context, req *TollReq, rsp *TollRsp) error {
	if req.Fail {
		return fmt.Errorf("the work did not happen")
	}
	rsp.Text = "done"
	return nil
}

func (Tolled) Free(_ context.Context, _ *TollReq, rsp *TollRsp) error {
	rsp.Text = "free"
	return nil
}

// ledger records what the gateway asked and was told.
type ledger struct {
	sync.Mutex
	allowed []string
	charged []string
	refuse  error
	free    bool
}

func (l *ledger) install(t *testing.T) {
	t.Helper()
	Gate.Allow = func(account, op string) (bool, error) {
		l.Lock()
		defer l.Unlock()
		l.allowed = append(l.allowed, account+" "+op)
		return !l.free, l.refuse
	}
	Gate.Charge = func(account, op string) {
		l.Lock()
		defer l.Unlock()
		l.charged = append(l.charged, account+" "+op)
	}
	t.Cleanup(func() { Gate.Allow, Gate.Charge = nil, nil })
}

func (l *ledger) counts() (int, int) {
	l.Lock()
	defer l.Unlock()
	return len(l.allowed), len(l.charged)
}

func registerTolled(t *testing.T, name string) {
	t.Helper()
	if _, already := SpecFor(name); already {
		return
	}
	err := Register(Spec{
		Name:    name,
		Handler: new(Tolled),
		Endpoints: map[string]Endpoint{
			"Priced": {Doc: "costs something", Cost: "test_op"},
			"Free":   {Doc: "costs nothing"},
		},
	})
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func callTolled(ctx context.Context, name, method string, req *TollReq) error {
	var rsp TollRsp
	return Call(ctx, name, "Tolled."+method, req, &rsp)
}

// TestAPricedCallIsAskedAboutAndCharged.
func TestAPricedCallIsAskedAboutAndCharged(t *testing.T) {
	const svc = "tolled-charged"
	registerTolled(t, svc)
	l := &ledger{}
	l.install(t)

	ctx := WithAccount(context.Background(), "asim")
	if err := callTolled(ctx, svc, "Priced", &TollReq{}); err != nil {
		t.Fatalf("call: %v", err)
	}
	allowed, charged := l.counts()
	if allowed != 1 {
		t.Errorf("the gateway was asked %d times, want 1", allowed)
	}
	if charged != 1 {
		t.Fatalf("the call was charged %d times, want 1 — this is the defect the "+
			"gateway exists for: a price declared and nothing debiting it", charged)
	}
	if l.charged[0] != "asim test_op" {
		t.Errorf("charged %q", l.charged[0])
	}
}

// TestAFreeCallIsNotAskedAbout — an endpoint with no Cost has no business
// demanding identity or consulting a wallet.
func TestAFreeCallIsNotAskedAbout(t *testing.T) {
	const svc = "tolled-free"
	registerTolled(t, svc)
	l := &ledger{}
	l.install(t)

	ctx := WithAccount(context.Background(), "asim")
	if err := callTolled(ctx, svc, "Free", &TollReq{}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if allowed, charged := l.counts(); allowed != 0 || charged != 0 {
		t.Errorf("a free method asked the gateway %d times and charged %d", allowed, charged)
	}
}

// TestARefusedCallDoesNotRun — the point of asking first.
func TestARefusedCallDoesNotRun(t *testing.T) {
	const svc = "tolled-refused"
	registerTolled(t, svc)
	l := &ledger{refuse: fmt.Errorf("this costs 5 credits and your balance is 0")}
	l.install(t)

	ctx := WithAccount(context.Background(), "skint")
	err := callTolled(ctx, svc, "Priced", &TollReq{})
	if err == nil {
		t.Fatal("a caller who cannot pay was served anyway")
	}
	if _, charged := l.counts(); charged != 0 {
		t.Errorf("a refused call was charged %d times", charged)
	}
}

// TestAFailedCallIsNotCharged. The provider may still have cost us, and
// charging for a failure loses an account rather than a credit.
func TestAFailedCallIsNotCharged(t *testing.T) {
	const svc = "tolled-failed"
	registerTolled(t, svc)
	l := &ledger{}
	l.install(t)

	ctx := WithAccount(context.Background(), "asim")
	if err := callTolled(ctx, svc, "Priced", &TollReq{Fail: true}); err == nil {
		t.Fatal("the handler was supposed to fail")
	}
	allowed, charged := l.counts()
	if allowed != 1 {
		t.Errorf("asked %d times", allowed)
	}
	if charged != 0 {
		t.Errorf("a failed call was charged %d times", charged)
	}
}

// TestAnAllowanceIsAllowedAndNotCharged — the shape free caps need: permitted
// and free are different answers from "has no price".
func TestAnAllowanceIsAllowedAndNotCharged(t *testing.T) {
	const svc = "tolled-allowance"
	registerTolled(t, svc)
	l := &ledger{free: true}
	l.install(t)

	ctx := WithAccount(context.Background(), "newcomer")
	if err := callTolled(ctx, svc, "Priced", &TollReq{}); err != nil {
		t.Fatalf("a call inside its allowance was refused: %v", err)
	}
	allowed, charged := l.counts()
	if allowed != 1 || charged != 0 {
		t.Errorf("asked %d, charged %d — an allowance should be asked about and not billed",
			allowed, charged)
	}
}

// TestAGuestReachesTheHandler — identity is the door's business, not the
// gateway's. A priced call with nobody to bill has already been let in.
func TestAGuestReachesTheHandler(t *testing.T) {
	const svc = "tolled-guest"
	registerTolled(t, svc)
	l := &ledger{}
	l.install(t)

	if err := callTolled(context.Background(), svc, "Priced", &TollReq{}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if allowed, charged := l.counts(); allowed != 0 || charged != 0 {
		t.Errorf("a call with no account asked %d and charged %d", allowed, charged)
	}
}

func TestMethodName(t *testing.T) {
	for in, want := range map[string]string{
		"Server.Search": "Search",
		"Tolled.Priced": "Priced",
		"Search":        "Search",
		"":              "",
	} {
		if got := methodName(in); got != want {
			t.Errorf("methodName(%q) = %q, want %q", in, got, want)
		}
	}
}
