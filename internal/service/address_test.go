package service

// Registering a service can fail for a reason that is nobody's fault.
//
// The in-process transport does not bind a socket. Given port 0 it picks a
// random number in 10000–29999 and keeps a map of what it handed out, so two
// services can be given the same address and the second fails with "already
// listening on 127.0.0.1:15804". Nothing is actually in use; two draws
// collided.
//
// An instance registers around twenty-five services at boot, which puts the
// odds of at least one collision near 1.5% per start — small per start, not
// small across a fleet, and the failure mode is a service that silently is not
// running. It showed up first as a flaky test, which is the cheap way to find
// out about it.

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type AddrProbeReq struct{}
type AddrProbeRsp struct {
	OK bool `json:"ok"`
}
type AddrProbeHandler struct{}

func (AddrProbeHandler) Ping(_ context.Context, _ *AddrProbeReq, rsp *AddrProbeRsp) error {
	rsp.OK = true
	return nil
}

// The retry only fires for the collision, and must not swallow a real failure.
func TestOnlyAnAddressCollisionIsRetried(t *testing.T) {
	if !isAddressTaken(errors.New("already listening on 127.0.0.1:15804")) {
		t.Error("the transport's collision was not recognised, so registration will fail on it")
	}
	for _, err := range []error{
		nil,
		errors.New("connection refused"),
		errors.New("service: handler has no exported methods"),
	} {
		if isAddressTaken(err) {
			t.Errorf("%v was treated as an address collision and would be retried forever", err)
		}
	}
}

// Registering many services must not fail.
//
// This is the collision made likely rather than possible: with n services the
// chance of a clash is about n²/2 over the 20000 addresses available, so a few
// hundred registrations all but guarantees one. Before the retry this failed;
// it cannot false-fail, because a run that happens to draw no collision still
// passes.
func TestManyServicesCanAllRegister(t *testing.T) {
	if testing.Short() {
		t.Skip("registers hundreds of services")
	}
	const n = 300
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("addrprobe%d", i)
		if err := Register(Spec{Name: name, Handler: AddrProbeHandler{}}); err != nil {
			t.Fatalf("registering %d services: %s failed at #%d: %v", n, name, i, err)
		}
	}
}
