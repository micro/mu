package sms

// Answering a text.
//
// The same seam as agent/mail and agent/chat: service/sms owns the number and
// may not call an agent, so it commits arrival facts with its own record. This consumer reads
// through the service API, checks ownership, and decides whether to answer.
//
// Only the verified owner of a number may use their assistant through it.
// Correspondents may reply to messages but cannot act as the account owner.

import (
	"context"
	"mu/internal/service"
	"strings"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/thread"
	svcsms "mu/service/sms"
)

// Client is which door this was.
const Client = thread.SMSClient

// clientFor is which door a channel is, in the record.
//
// Its own client rather than one with a flag: the reply has to go back the same
// way it came, and a WhatsApp conversation answered by text lands on the other
// person's phone as a second thread from a number they do not recognise.
func clientFor(channel svcsms.Channel) string {
	if channel == svcsms.ChannelWhatsApp {
		return thread.WhatsAppClient
	}
	return Client
}

// Load consumes stored arrivals. The trust fact is captured at receipt and
// ownership is checked again before the account's assistant is invoked.
func Load() { agent.WatchEvents("agent-sms", []string{"sms.created"}, prepareText) }

func prepareText(e event.Record) (func(), error) {
	if e.Data["collection"] != "messages" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var rsp service.SourceResponse
	if err := service.Call(service.WithAccount(ctx, e.Account), "sms", "Server.Source", &service.SourceRequest{ID: e.Resource}, &rsp); err != nil {
		return nil, err
	}
	m := rsp.Item
	if m == nil || m.Direction != "in" || m.Facts["verified_owner"] != true {
		return nil, nil
	}
	owner, verified := svcsms.KnownSender(m.From)
	if !verified || owner != e.Account {
		return nil, nil
	}
	t := texted{ID: m.ID, Owner: e.Account, From: m.From, Text: m.Text, Channel: svcsms.Channel(m.Channel)}
	return func() { answer(t) }, nil
}

// texted is one message that is expecting an answer.
type texted struct {
	ID      string
	Owner   string
	From    string
	Text    string
	Channel svcsms.Channel
}

// answer asks, then texts back.
func answer(t texted) {
	owner, verified := svcsms.KnownSender(t.From)
	if !verified || owner != t.Owner {
		return
	}
	res, err := agent.Ask(agent.AskRequest{
		Account:    t.Owner,
		MessageRef: t.ID,
		Client:     clientFor(t.Channel),
		// The number is the conversation, so a second text continues the first.
		// A phone has no threads; the person on the other end is the thread.
		Thread:  t.From,
		Text:    t.Text,
		Trigger: "sms",
	})
	if err != nil {
		app.Log("sms", "agent could not answer %s: %v", t.From, err)
		return
	}
	reply := strings.TrimSpace(res.Text)
	if reply == "" {
		return
	}

	// Send refuses anything over three segments, which an agent will exceed
	// without trying. Trimmed here rather than left to fail, because a refusal
	// upstream is silence on the phone and silence reads as broken.
	//
	// Per channel, because the limits are not the same thing: a text is
	// rationed by what a segment costs, and a WhatsApp message costs one
	// conversation however long it is. Trimming a WhatsApp reply to 460
	// characters would cut an answer short for a reason that does not apply.
	if limit := replyLimitFor(t.Channel); len(reply) > limit {
		reply = strings.TrimSpace(reply[:limit-1]) + "…"
	}
	if _, err := svcsms.SendOn(t.Channel, t.Owner, t.From, reply); err != nil {
		app.Log("sms", "could not answer %s on %s: %v", t.From, t.Channel.Label(), err)
	}
}

// replyLimit is how much of an answer fits in a text.
//
// Under service/sms's own maximum, because that one is a refusal and this is a
// trim: an answer that arrives shortened is worth more than one that does not
// arrive. Somebody who wants the whole thing has the same conversation waiting
// in their inbox, which is the point of one record across every door.
const replyLimit = 460

// replyLimitFor is how much of an answer fits on a channel.
//
// WhatsApp carries 4096 characters in one message at one price, so the only
// reason to trim is that nobody reads an essay on a phone.
func replyLimitFor(channel svcsms.Channel) int {
	if channel == svcsms.ChannelWhatsApp {
		return whatsAppReplyLimit
	}
	return replyLimit
}

// whatsAppReplyLimit is well under the protocol's 4096. The limit here is
// attention rather than money.
const whatsAppReplyLimit = 1500
