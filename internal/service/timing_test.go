package service

import (
	"context"
	"errors"
	"go-micro.dev/v6/client"
	"testing"
	"time"
)

type timingStub struct {
	client.Client
	err error
}

func (c timingStub) Call(context.Context, client.Request, interface{}, ...client.CallOption) error {
	return c.err
}
func TestTimingPreservesServiceErrors(t *testing.T) {
	old := OnCallTiming
	defer func() { OnCallTiming = old }()
	expected := errors.New("failed")
	calls := 0
	OnCallTiming = func(s, m string, d time.Duration, e error) {
		calls++
		if s != "test" || m != "Test.Read" || e != expected || d < 0 {
			t.Fatal("incorrect timing")
		}
	}
	base := client.NewClient()
	c := timingClientWrapper(timingStub{base, expected})
	if err := c.Call(context.Background(), base.NewRequest("test", "Test.Read", nil), nil); err != expected || calls != 1 {
		t.Fatal("call changed")
	}
}
