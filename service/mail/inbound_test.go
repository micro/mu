package mail

// What registering for an address has to guarantee.

import (
	"sync"
	"testing"
	"time"
)

// noHandlers clears the registry for the duration of a test.
func noHandlers(t *testing.T) {
	t.Helper()
	inboundMu.Lock()
	prev := inboundHandlers
	inboundHandlers = map[string][]InboundHandler{}
	inboundMu.Unlock()
	t.Cleanup(func() {
		inboundMu.Lock()
		inboundHandlers = prev
		inboundMu.Unlock()
	})
}

func TestTwoHandlersForOneAddressBothRun(t *testing.T) {
	// Nothing owns an address. Registering second must not silently displace
	// whatever was there first, which is the failure mode of the function
	// variable this replaced.
	noHandlers(t)

	var mu sync.Mutex
	var got []string
	done := make(chan struct{}, 2)
	Inbound(Tagged, func(InboundMail) {
		mu.Lock()
		got = append(got, "first")
		mu.Unlock()
		done <- struct{}{}
	})
	Inbound(Tagged, func(InboundMail) {
		mu.Lock()
		got = append(got, "second")
		mu.Unlock()
		done <- struct{}{}
	})

	hs := handlersFor(InboundMail{Tag: "research"})
	if len(hs) != 2 {
		t.Fatalf("registered two handlers, found %d", len(hs))
	}
	for _, h := range hs {
		go h(InboundMail{})
	}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a handler never ran")
		}
	}
	if len(got) != 2 {
		t.Errorf("ran %v, want both", got)
	}
}

func TestTaggedAndSharedAreDifferentAddresses(t *testing.T) {
	noHandlers(t)

	Inbound(Tagged, func(InboundMail) {})
	if hs := handlersFor(InboundMail{Shared: true}); len(hs) != 0 {
		t.Errorf("mail to agent@ reached a handler registered for tagged mail")
	}
	if hs := handlersFor(InboundMail{Tag: "research"}); len(hs) != 1 {
		t.Errorf("tagged mail did not reach its handler")
	}

	Inbound(AgentMailbox, func(InboundMail) {})
	if hs := handlersFor(InboundMail{Shared: true}); len(hs) != 1 {
		t.Errorf("mail to agent@ did not reach its handler")
	}
}

func TestRegisteringNonsenseIsIgnored(t *testing.T) {
	noHandlers(t)

	Inbound("", func(InboundMail) {})
	Inbound("agent", nil)
	if anyRegistered() {
		t.Error("an empty address or a nil handler was registered")
	}
}

func TestAHandlerThatPanicsDoesNotTakeTheServerWithIt(t *testing.T) {
	// A handler is somebody else's code running on the mail path.
	noHandlers(t)

	survived := make(chan struct{})
	Inbound(Tagged, func(InboundMail) { panic("handler is broken") })
	Inbound(Tagged, func(InboundMail) { close(survived) })

	withDomain(t, "micro.mu")
	prevKnown := KnownSender
	KnownSender = func(string, string) bool { return true }
	t.Cleanup(func() { KnownSender = prevKnown })

	deliverInbound(
		InboundMail{Owner: "o", Tag: "research", From: "asim@aslam.me"},
		wakeRequest{Owner: "o", Tag: "research", From: "asim@aslam.me", Authenticated: true},
	)

	select {
	case <-survived:
	case <-time.After(2 * time.Second):
		t.Fatal("one handler panicking stopped the others")
	}
}

func TestNothingIsDispatchedToWhenTheGuardSaysNo(t *testing.T) {
	noHandlers(t)

	ran := make(chan struct{}, 1)
	Inbound(Tagged, func(InboundMail) { ran <- struct{}{} })

	withDomain(t, "micro.mu")
	// Everybody is known, so only the guard's other reasons can refuse.
	prevKnown := KnownSender
	KnownSender = func(string, string) bool { return true }
	t.Cleanup(func() { KnownSender = prevKnown })
	// Spam, and an unauthenticated sender: either is enough on its own.
	deliverInbound(
		InboundMail{Owner: "o", Tag: "research"},
		wakeRequest{Owner: "o", Tag: "research", From: "spammer@example.com", IsSpam: true, Authenticated: true},
	)
	deliverInbound(
		InboundMail{Owner: "o", Tag: "research"},
		wakeRequest{Owner: "o", Tag: "research", From: "stranger@example.com", Authenticated: false},
	)

	select {
	case <-ran:
		t.Error("a handler ran for mail the guard refused")
	case <-time.After(300 * time.Millisecond):
	}
}
