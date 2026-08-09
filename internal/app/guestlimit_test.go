package app

// The limit that makes an open door safe to leave open.
//
// Free tools answer callers with no account, which is the point — an agent
// finds the endpoint mid-task, tries something, and it works. The credit charge
// is no defence there, because the charge is zero. This is the only thing
// standing between a free tool and a loop, so it has to actually count, reset,
// and be generous enough that a person wiring up an agent never meets it.

import (
	"fmt"
	"testing"
	"time"
)

func TestGuestCallsAreCountedPerAddressAndCapped(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "3")
	t.Setenv("GUEST_WINDOW_MINUTES", "60")
	resetGuestCalls()

	for i := 1; i <= 3; i++ {
		if !GuestCallAllowed("198.51.100.7") {
			t.Fatalf("call %d was refused inside the limit", i)
		}
	}
	if GuestCallAllowed("198.51.100.7") {
		t.Error("the limit does not stop anything — a loop runs forever")
	}

	// Per address, so one caller cannot exhaust the allowance for everyone.
	if !GuestCallAllowed("203.0.113.9") {
		t.Error("a different address was refused because of somebody else's calls")
	}
}

// Localhost is never limited: a self-hosted instance is talking to itself, and
// rationing that is rationing the operator.
func TestLocalhostIsNeverLimited(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "1")
	resetGuestCalls()

	for _, ip := range []string{"127.0.0.1", "::1", ""} {
		for i := 0; i < 5; i++ {
			if !GuestCallAllowed(ip) {
				t.Errorf("%q was rate limited on call %d", ip, i+1)
				break
			}
		}
	}
}

// The window resets, or the first busy hour would lock an address out forever.
func TestTheAllowanceComesBack(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "2")
	t.Setenv("GUEST_WINDOW_MINUTES", "60")
	resetGuestCalls()

	const ip = "192.0.2.44"
	GuestCallAllowed(ip)
	GuestCallAllowed(ip)
	if GuestCallAllowed(ip) {
		t.Fatal("the limit did not engage")
	}

	expireGuestBucket(ip)
	if !GuestCallAllowed(ip) {
		t.Error("the allowance never comes back, so one busy hour bans an address permanently")
	}
}

// The default has to be generous. Somebody wiring up an agent tries the same
// call repeatedly while they get it working, and a limit that trips during
// evaluation is indistinguishable from a broken endpoint.
func TestTheDefaultAllowanceIsGenerous(t *testing.T) {
	resetGuestCalls()
	const ip = "192.0.2.99"
	for i := 0; i < 100; i++ {
		if !GuestCallAllowed(ip) {
			t.Fatalf("the default limit refused call %d — too tight to evaluate against", i+1)
		}
	}
}

// A bucket per address is a leak per address unless the map is swept. The
// sweep only removes windows that have elapsed — a live bucket cannot be
// evicted without giving away somebody's allowance — so this expires them
// first and then checks they actually go.
func TestExpiredBucketsAreSweptAway(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "1")
	resetGuestCalls()

	const n = 10100
	for i := 0; i < n; i++ {
		GuestCallAllowed(fmt.Sprintf("198.51.%d.%d", i/250, i%250))
	}
	guestMu.Lock()
	before := len(guestCalls)
	for _, b := range guestCalls {
		b.resetAt = time.Now().Add(-time.Minute)
	}
	guestMu.Unlock()
	if before < n {
		t.Fatalf("only %d buckets were created, expected %d", before, n)
	}

	// One more call past the threshold triggers the sweep.
	GuestCallAllowed("203.0.113.200")

	guestMu.Lock()
	after := len(guestCalls)
	guestMu.Unlock()
	if after >= before {
		t.Errorf("%d buckets before, %d after expiring them all — nothing is swept, "+
			"so the map grows with every address that ever called", before, after)
	}
}
