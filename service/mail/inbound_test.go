package mail

// What arrived, and what may wake something, are two questions — and they are
// two topics, so the second cannot be answered by forgetting to ask.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/event"
)

// onTopic collects the messages published to a topic during a test.
func onTopic(t *testing.T, topic string) <-chan InboundMail {
	t.Helper()
	sub := event.Subscribe(topic)
	t.Cleanup(sub.Close)

	out := make(chan InboundMail, 8)
	go func() {
		for e := range sub.Chan {
			if m, ok := MessageFrom(e.Data); ok {
				out <- m
			}
		}
	}()
	return out
}

func waited(ch <-chan InboundMail) (InboundMail, bool) {
	select {
	case m := <-ch:
		return m, true
	case <-time.After(2 * time.Second):
		return InboundMail{}, false
	}
}

func quiet(ch <-chan InboundMail) bool {
	select {
	case <-ch:
		return false
	case <-time.After(300 * time.Millisecond):
		return true
	}
}

// A stranger's mail is still mail you were sent.
//
// The gate is about trust: nobody who is not in your address book gets to drive
// your agent and spend your credits. It was also, by accident, the only way
// anything ever reached the record — so an account with a full mailbox had an
// empty /inbox, and the agent could not see the thing it was supposed to be
// working on.
//
// Both halves of the SMTP path, because that is what it does: the message is
// stored, which announces the arrival, and then the gate is asked separately
// whether anything may act on it.
func TestEveryDeliveryIsAnnouncedEvenWhenNothingMayBeWoken(t *testing.T) {
	arrived := onTopic(t, event.EventMailReceived)
	wake := onTopic(t, event.EventMailForAgent)

	if err := SendMessageTo(Delivery{
		FromID: "stranger@example.com", ToID: "acc", Subject: "hello", Body: "hi",
	}); err != nil {
		t.Fatalf("storing the message: %v", err)
	}
	// Untagged mail from somebody this account has never heard of: exactly the
	// message mayDispatch refuses.
	deliverInbound(
		InboundMail{Owner: "acc", From: "stranger@example.com", Subject: "hello"},
		wakeRequest{Owner: "acc", From: "stranger@example.com"},
	)

	m, ok := waited(arrived)
	if !ok {
		t.Fatal("a delivery nothing may act on was never announced")
	}
	if m.Subject != "hello" {
		t.Errorf("the message arrived as %q", m.Subject)
	}
	if !quiet(wake) {
		t.Error("a stranger's mail was published as wakeable")
	}
}

// Storing a message is what announces it, whichever door it came in by.
//
// This is the bug that made the split worth finding: an admin alert, an invite
// and a verification mail are all delivered locally, never touch SMTP, and so
// never reached internal/thread. They were in /mail and in IMAP and absent from
// /inbox — the page whose whole claim is that things turn up in it.
//
// The assertion is about the shape as much as the fact. Two publishers were
// putting two different payloads on this one topic, and each subscriber
// understood one of them, so "was it announced" and "could anyone read it"
// were different questions.
func TestLocalDeliveryReachesTheRecord(t *testing.T) {
	withDomain(t, "mu.test")
	arrived := onTopic(t, event.EventMailReceived)

	if err := SendMessageTo(Delivery{
		From: "Mu", FromID: "no-reply@mu.test",
		ToID: "acc", Tag: "research",
		Subject: "Disk is 91% full", Body: "<p>go and look</p>",
		MessageID: "<alert@mu.test>", InReplyTo: "<earlier@mu.test>",
	}); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	m, ok := waited(arrived)
	if !ok {
		t.Fatal("mail delivered locally was never announced, so it cannot reach /inbox")
	}
	// Everything the recorder needs: whose it is, what it says, and enough to
	// put it on the conversation it belongs to.
	if m.Owner != "acc" {
		t.Errorf("Owner = %q, so the record does not know whose mail this is", m.Owner)
	}
	if m.Subject != "Disk is 91% full" {
		t.Errorf("Subject = %q", m.Subject)
	}
	// Trimmed here, as the recorder trims it: what matters is that the prose
	// is what travels and not the markup the message was stored as.
	if strings.TrimSpace(m.Text) != "go and look" {
		t.Errorf("Text = %q, want the prose — the agent is handed this one", m.Text)
	}
	if m.From != "no-reply@mu.test" || m.FromName != "Mu" {
		t.Errorf("sender arrived as %q/%q", m.FromName, m.From)
	}
	if m.To != "acc+research@mu.test" {
		t.Errorf("To = %q, want the address it was delivered to", m.To)
	}
	if m.MessageID != "<alert@mu.test>" || m.InReplyTo != "<earlier@mu.test>" {
		t.Errorf("threading arrived as %q/%q — a reply would start a new "+
			"conversation instead of joining one", m.MessageID, m.InReplyTo)
	}
}

// Spam is the one thing that does not go in. It was refused at the door in
// every sense that matters, and a record full of it is not a record.
func TestSpamIsNotAnnouncedAtAll(t *testing.T) {
	arrived := onTopic(t, event.EventMailReceived)
	wake := onTopic(t, event.EventMailForAgent)

	deliverInbound(
		InboundMail{Owner: "acc", From: "spam@example.com"},
		wakeRequest{Owner: "acc", From: "spam@example.com", IsSpam: true},
	)

	if !quiet(arrived) {
		t.Error("spam reached the record")
	}
	if !quiet(wake) {
		t.Error("spam was published as wakeable")
	}
}

// The gate decides what is published, not what a subscriber does about it.
// Nothing downstream can answer a message the guard refused, because the
// message never reached the topic it would have to arrive on.
func TestTheGuardDecidesWhatIsPublished(t *testing.T) {
	withDomain(t, "micro.mu")
	// Everybody is known, so only the guard's other reasons can refuse.
	prevKnown := KnownSender
	KnownSender = func(string, string) bool { return true }
	t.Cleanup(func() { KnownSender = prevKnown })

	wake := onTopic(t, event.EventMailForAgent)

	// An unauthenticated sender — enough on its own.
	deliverInbound(
		InboundMail{Owner: "o", Tag: "research"},
		wakeRequest{Owner: "o", Tag: "research", From: "stranger@example.com", Authenticated: false},
	)
	if !quiet(wake) {
		t.Error("unauthenticated mail was published as wakeable")
	}

	// And the same message, authenticated, is.
	deliverInbound(
		InboundMail{Owner: "o", Tag: "research", Subject: "let through"},
		wakeRequest{Owner: "o", Tag: "research", From: "asim@aslam.me", Authenticated: true},
	)
	m, ok := waited(wake)
	if !ok {
		t.Fatal("authenticated tagged mail from a known sender was refused too, so " +
			"this test is asserting that nothing ever passes")
	}
	if m.Subject != "let through" {
		t.Errorf("the wrong message came through: %q", m.Subject)
	}
}

// Two subscribers on one topic both get the message. Nothing owns a topic,
// which is what the registry this replaced had to be careful about — the
// function variable *it* replaced silently displaced whatever was there first.
func TestTwoSubscribersBothGetIt(t *testing.T) {
	first := onTopic(t, event.EventMailReceived)
	second := onTopic(t, event.EventMailReceived)

	if err := SendMessageTo(Delivery{
		FromID: "someone@example.com", ToID: "acc", Subject: "both", Body: "x",
	}); err != nil {
		t.Fatalf("storing the message: %v", err)
	}

	for i, ch := range []<-chan InboundMail{first, second} {
		if m, ok := waited(ch); !ok || m.Subject != "both" {
			t.Errorf("subscriber %d did not get the message", i+1)
		}
	}
}

// The message survives the trip. It travels as JSON on a bus that carries
// map[string]interface{}, so a field is one missing tag away from arriving
// zeroed — and Others being dropped would turn a Cc'd thread back into a
// one-to-one reply, silently.
func TestTheWholeMessageSurvivesTheBus(t *testing.T) {
	withDomain(t, "micro.mu")
	prevKnown := KnownSender
	KnownSender = func(string, string) bool { return true }
	t.Cleanup(func() { KnownSender = prevKnown })

	arrived := onTopic(t, event.EventMailForAgent)

	sent := InboundMail{
		Owner: "acc", Tag: "research", Shared: true,
		From: "asim@aslam.me", To: "acc+research@mu.test", FromName: "Asim",
		Subject: "the whole thing", Body: "<p>hi</p>", Text: "hi",
		Attachment: "report.zip",
		Others:     []string{"someone@example.com", "other@example.com"},
		MessageID:  "<a@b>", InReplyTo: "<c@d>", References: "<e@f>",
	}
	deliverInbound(sent, wakeRequest{Owner: "acc", Tag: "research",
		From: sent.From, Authenticated: true})

	got, ok := waited(arrived)
	if !ok {
		t.Fatal("nothing arrived")
	}
	if got.Owner != sent.Owner || got.Tag != sent.Tag || got.Shared != sent.Shared ||
		got.From != sent.From || got.To != sent.To || got.FromName != sent.FromName ||
		got.Subject != sent.Subject || got.Body != sent.Body || got.Text != sent.Text ||
		got.Attachment != sent.Attachment || got.MessageID != sent.MessageID ||
		got.InReplyTo != sent.InReplyTo || got.References != sent.References {
		t.Errorf("the message changed on the way:\nsent %+v\ngot  %+v", sent, got)
	}
	if len(got.Others) != len(sent.Others) {
		t.Fatalf("Others arrived as %v, want %v — a Cc'd thread would answer one person",
			got.Others, sent.Others)
	}
	for i := range sent.Others {
		if got.Others[i] != sent.Others[i] {
			t.Errorf("Others[%d] = %q, want %q", i, got.Others[i], sent.Others[i])
		}
	}
}
