package stream

// The whole package rests on one round trip: a service calls event.Announce
// and this one is subscribed on the other side. Nothing in the type system
// holds that together — the payload is a map, the topic is a string, and a
// renamed key or an unsubscribed topic fails by the timeline simply staying
// empty, which looks exactly like nothing having happened yet.
//
// So this drives the real broker rather than calling add directly.

import (
	"testing"
	"time"

	"mu/internal/event"
)

func TestAnAnnouncedFactReachesTheTimeline(t *testing.T) {
	reset(t)

	event.Announce("blog", "New post: Hello", "/blog/post?id=1", "")

	if !eventually(func() bool {
		for _, e := range Recent(10, "") {
			if e.Service == "blog" && e.Text == "New post: Hello" && e.URL == "/blog/post?id=1" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("a fact announced on the bus never reached the timeline")
	}
}

// The same trip for the one fact stream did not have to ask for: mail has
// announced its own arrival since the hook into the agent was deleted, and an
// entry made from it must be its owner's alone.
func TestMailArrivingIsTheOwnersAlone(t *testing.T) {
	reset(t)

	event.Publish(event.Event{Type: event.EventMailReceived, Data: map[string]interface{}{
		"account": "alice",
		"from":    "lawyer@example.com",
		"subject": "Re: the settlement",
	}})

	if !eventually(func() bool { return len(Recent(10, "alice")) > 0 }) {
		t.Fatal("mail arriving never reached the timeline")
	}
	if got := Recent(10, "bob"); len(got) != 0 {
		t.Fatalf("somebody else can see alice's mail: %v", got)
	}
}

// An announcement with no account owner is a publication, so mail with no
// account named must not become one.
func TestMailWithNoOwnerIsNotPublished(t *testing.T) {
	reset(t)

	event.Publish(event.Event{Type: event.EventMailReceived, Data: map[string]interface{}{
		"from":    "lawyer@example.com",
		"subject": "Re: the settlement",
	}})

	time.Sleep(200 * time.Millisecond)
	if got := Recent(10, ""); len(got) != 0 {
		t.Fatalf("ownerless mail was published to everybody: %v", got)
	}
}

func eventually(ok func() bool) bool {
	for range 100 {
		if ok() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
