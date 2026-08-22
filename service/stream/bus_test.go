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

// Mail arriving is not this page's business, on either topic.
//
// There were two tests here and they asserted the opposite: that mail reached
// the timeline, and that ownerless mail did not. Both passed while the feature
// was dead, because each hand-wrote the event in the shape the *subscriber*
// expected rather than the shape service/mail actually publishes — and the
// publisher had moved to a single "message" key. The comment at the top of this
// file describes that failure exactly ("a renamed key... fails by the timeline
// simply staying empty") and then the tests wrote the key themselves, which is
// the one thing that makes the round trip unable to prove anything.
//
// So this asserts the rule instead of the wiring, and it does it with the real
// payload: nothing about somebody's mail belongs on a page served without a
// session. See theirsAlone.
func TestMailDoesNotReachTheTimeline(t *testing.T) {
	reset(t)

	for _, topic := range []string{event.EventMailReceived, event.EventMailForAgent} {
		// The shape service/mail sends, and the shape it used to send. Neither
		// may put a row on the timeline.
		event.Publish(event.Event{Type: topic, Data: map[string]interface{}{
			"message": `{"To":"alice","From":"lawyer@example.com","Subject":"Re: the settlement"}`,
		}})
		event.Publish(event.Event{Type: topic, Data: map[string]interface{}{
			"account": "alice",
			"from":    "lawyer@example.com",
			"subject": "Re: the settlement",
		}})
	}

	time.Sleep(200 * time.Millisecond)
	for _, viewer := range []string{"", "alice"} {
		if got := Recent(10, viewer); len(got) != 0 {
			t.Fatalf("mail reached the timeline for viewer %q: %v", viewer, got)
		}
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
