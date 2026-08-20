package mail

// Everything that arrives is in the record, and arriving once is one message.

import (
	"strings"
	"testing"

	"mu/internal/thread"
	"mu/service/mail"
)

// Ordinary mail — nothing addressed to an agent, from somebody who could never
// wake one — is in the inbox.
//
// This is the fact the page was missing. /inbox reads the record, the record was
// only ever written by the handler that answers agent mail, so an account with a
// full mailbox saw an empty list.
func TestOrdinaryMailIsInTheRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "record-plain"
	recordDelivery(mail.InboundMail{
		Owner:     who,
		From:      "accounts@supplier.example",
		FromName:  "Supplier Accounts",
		To:        "you@micro.mu",
		Subject:   "Invoice 4021",
		Text:      "Attached is this month's invoice.",
		MessageID: "<inv4021@supplier.example>",
	})

	threads := thread.List(who, 10)
	if len(threads) != 1 {
		t.Fatalf("%d conversations, want 1", len(threads))
	}
	msgs := thread.Messages(who, threads[0].ID, 10)
	if len(msgs) != 1 {
		t.Fatalf("%d messages, want 1", len(msgs))
	}
	// What was written, without the subject in front of it. The subject is the
	// conversation's, not every message's — see thread.Name.
	if msgs[0].Text != "Attached is this month's invoice." {
		t.Errorf("the message is not what arrived: %q", msgs[0].Text)
	}
	if strings.Contains(msgs[0].Text, "Invoice 4021") {
		t.Error("the subject is inside the message, so a reader meets it as the " +
			"heading and again at the top of every message under it")
	}
	if threads[0].Subject != "Invoice 4021" {
		t.Errorf("the conversation is called %q, not what the mail was about", threads[0].Subject)
	}
	// Its own id, so a reply to it finds this conversation and so that the
	// agent recording the same arrival does not record it twice.
	if msgs[0].Ref != "<inv4021@supplier.example>" {
		t.Errorf("the message is filed under %q", msgs[0].Ref)
	}
	// And who wrote in, by name — which belongs to the party, not to each line.
	var named bool
	for _, p := range thread.Parties(who, threads[0].ID) {
		if p.Key == "accounts@supplier.example" && p.Name == "Supplier Accounts" {
			named = true
		}
	}
	if !named {
		t.Error("nobody is on the conversation by name")
	}
}

// The recorder and the agent both see the same arrival, on separate goroutines,
// either order. It is one message.
func TestRecordingAnArrivalTwiceIsOneMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "record-twice"
	m := mail.InboundMail{
		Owner:     who,
		From:      "asim@aslam.me",
		To:        "agent@micro.mu",
		Subject:   "what are the markets doing",
		Text:      "and anything worth knowing",
		MessageID: "<ask1@phone>",
	}

	// The recorder.
	recordDelivery(m)
	// And the agent's own door, which is what Ask calls on the way in.
	th := thread.Open(who, Client, chainKey(m))
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: asked(m), Ref: m.MessageID, From: m.From})

	if threads := thread.List(who, 10); len(threads) != 1 {
		t.Fatalf("%d conversations, want 1", len(threads))
	}
	if msgs := thread.Messages(who, th.ID, 10); len(msgs) != 1 {
		t.Fatalf("one arrival left %d messages, want 1", len(msgs))
	}
}

// A reply lands on the conversation it answers rather than starting another.
func TestAReplyLandsOnTheSameConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const who = "record-chain"
	recordDelivery(mail.InboundMail{
		Owner: who, From: "a@example.com", To: "you@micro.mu",
		Subject: "lunch", Text: "thursday?", MessageID: "<one@example.com>",
	})
	recordDelivery(mail.InboundMail{
		Owner: who, From: "a@example.com", To: "you@micro.mu",
		Subject: "Re: lunch", Text: "or friday", MessageID: "<two@example.com>",
		InReplyTo: "<one@example.com>", References: "<one@example.com>",
	})

	threads := thread.List(who, 10)
	if len(threads) != 1 {
		t.Fatalf("a reply started a second conversation: %d in all", len(threads))
	}
	if msgs := thread.Messages(who, threads[0].ID, 10); len(msgs) != 2 {
		t.Fatalf("%d messages on the chain, want 2", len(msgs))
	}
}
