package mail

// What arrived, and who may wake something with it, are two questions.

import (
	"sync"
	"testing"
	"time"
)

// noDelivered clears the delivery registry for the duration of a test.
func noDelivered(t *testing.T) {
	t.Helper()
	inboundMu.Lock()
	prev := deliverHandlers
	deliverHandlers = nil
	inboundMu.Unlock()
	t.Cleanup(func() {
		inboundMu.Lock()
		deliverHandlers = prev
		inboundMu.Unlock()
	})
}

// A stranger's mail is still mail you were sent.
//
// The gate on Inbound is about trust: nobody who is not in your address book
// gets to drive your agent and spend your credits. It was also, by accident, the
// only way anything ever reached the record — so an account with a full mailbox
// had an empty /inbox, and the agent could not see the thing it was supposed to
// be working on.
func TestEveryDeliveryIsHandedOnEvenWhenNothingMayBeWoken(t *testing.T) {
	noHandlers(t)
	noDelivered(t)

	woke := make(chan struct{}, 1)
	Inbound(Tagged, func(InboundMail) { woke <- struct{}{} })

	arrived := make(chan InboundMail, 1)
	Delivered(func(m InboundMail) { arrived <- m })

	// Untagged mail from somebody this account has never heard of: exactly the
	// message mayDispatch refuses.
	deliverInbound(
		InboundMail{Owner: "acc", From: "stranger@example.com", Subject: "hello"},
		wakeRequest{Owner: "acc", From: "stranger@example.com"},
	)

	select {
	case m := <-arrived:
		if m.Subject != "hello" {
			t.Errorf("the handler got %q", m.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a delivery nothing may act on was never recorded")
	}

	select {
	case <-woke:
		t.Error("a stranger woke an agent")
	case <-time.After(100 * time.Millisecond):
	}
}

// Spam is the one thing that does not go in. It was refused at the door in
// every sense that matters, and a record full of it is not a record.
func TestSpamIsNotRecorded(t *testing.T) {
	noHandlers(t)
	noDelivered(t)

	var mu sync.Mutex
	seen := 0
	Delivered(func(InboundMail) {
		mu.Lock()
		seen++
		mu.Unlock()
	})

	deliverInbound(
		InboundMail{Owner: "acc", From: "spam@example.com"},
		wakeRequest{Owner: "acc", From: "spam@example.com", IsSpam: true},
	)

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if seen != 0 {
		t.Errorf("spam was handed on %d times", seen)
	}
}
