package mail

// The same message twice is one message.

import (
	"strings"
	"testing"

	"mu/internal/event"
)

// A retried delivery does not arrive twice.
//
// A sending server retries when a delivery is not cleanly acknowledged, which
// is what a restart mid-session looks like from the outside. Nothing checked,
// so the retry was stored again with a new id — and because arrival is
// announced from the store, it also rang the phone a second time and forwarded
// a second copy to the account's real address. Reported as a DMARC report
// arriving twice with nothing new in the inbox to explain it.
func TestARetriedDeliveryIsStoredOnce(t *testing.T) {
	who := "dupe-" + t.Name()
	d := Delivery{
		From: "noreply@google.com", FromID: "noreply@google.com",
		To: who, ToID: who,
		Subject:   "Report domain: micro.mu",
		Body:      "[report.zip (application/zip), 684 bytes — not shown]",
		MessageID: "<dmarc-1788307200@google.com>",
	}

	arrivals := event.Subscribe(event.MailReceived)
	defer arrivals.Close()

	if err := SendMessageTo(d); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := SendMessageTo(d); err != nil {
		t.Fatalf("the retry was rejected rather than ignored: %v", err)
	}

	if n := countFor(who, d.MessageID); n != 1 {
		t.Errorf("%d copies stored, want 1", n)
	}
	// And announced once, which is what decides how many notifications and
	// forwarded copies go out.
	if n := drain(arrivals, who); n != 1 {
		t.Errorf("arrival announced %d times, want 1", n)
	}
}

// The same id to a different person is a different delivery — a list, or a Cc.
func TestTheSameIdToSomebodyElseStillArrives(t *testing.T) {
	id := "<list-1788307200@example.test>"
	a, b := "dupe-a-"+t.Name(), "dupe-b-"+t.Name()
	for _, who := range []string{a, b} {
		if err := SendMessageTo(Delivery{
			From: "list@example.test", FromID: "list@example.test",
			To: who, ToID: who, Subject: "Notice", Body: "x", MessageID: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := countFor(a, id); n != 1 {
		t.Errorf("the first recipient has %d copies", n)
	}
	if n := countFor(b, id); n != 1 {
		t.Errorf("the second recipient has %d copies — their mail was swallowed as a duplicate", n)
	}
}

// And a delivery with no Message-ID is never a duplicate of anything.
//
// An id is what makes two arrivals the same arrival. Without one the honest
// answer is that this is new mail, and several local senders set none.
func TestMailWithNoIdIsAlwaysNew(t *testing.T) {
	who := "dupe-noid-" + t.Name()
	for i := 0; i < 2; i++ {
		if err := SendMessageTo(Delivery{
			From: "a@example.test", FromID: "a@example.test",
			To: who, ToID: who, Subject: "Hello", Body: "again",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := countFor(who, ""); n != 2 {
		t.Errorf("%d messages stored, want 2 — mail with no id was deduped against itself", n)
	}
}

// countFor is how many of this account's messages carry the given header id.
func countFor(who, messageID string) int {
	mutex.RLock()
	defer mutex.RUnlock()
	n := 0
	for _, m := range messages {
		if m == nil || !strings.EqualFold(m.ToID, who) {
			continue
		}
		if m.MessageID == messageID {
			n++
		}
	}
	return n
}

// drain counts the arrivals announced for an account, without waiting for ones
// that are not coming.
func drain(sub *event.Subscription, who string) int {
	n := 0
	for {
		select {
		case e, ok := <-sub.Chan:
			if !ok {
				return n
			}
			if m, ok := MessageFrom(e.Data); ok && strings.EqualFold(m.Owner, who) {
				n++
			}
		default:
			return n
		}
	}
}
