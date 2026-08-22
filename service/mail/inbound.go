package mail

// Saying that mail arrived.
//
// This service used to know what an agent was. There was a `var InboundAgent
// func(InboundMail)` here, filled in from the server's wiring, and a rule
// called shouldWakeAgent — a mail server with a special case for one feature of
// the product on top of it. That became a registration: something said it
// handled mail at an address and this dispatched to it.
//
// A registration is still a call, and it left two mechanisms for one fact —
// this registry, and event.EventMailReceived on the bus, which internal/event
// already describes correctly: "Mail arriving is a fact, not a call. Anything
// that wants to act on it subscribes." So the registry is gone and there is one
// mechanism. Nothing here knows whether anything is listening.
//
// What stays is the part that is genuinely mail's business: whether the person
// who wrote in is entitled to be listened to at all. That guard is not a
// subscriber's to make — it needs the SPF and DKIM results, which exist only
// inside the SMTP session, and it is the same question whoever is listening.
//
// It is enforced by which topic a message is published on rather than by a
// field on it. See event.EventMailForAgent for why.

import (
	"encoding/json"

	"mu/internal/app"
	"mu/internal/event"
)

// Tagged is the address every account has a private version of: you+anything@.
// Kept as the name of that shape, which cc.go and the agent roster both read.
const Tagged = "+"

// deliverInbound says a message arrived, and separately says whether it may
// wake an agent.
//
// Called after the message is saved, so the mail is in the inbox whether or not
// anything acts on it.
//
// Two publications, not one with a flag. The first is a fact — this arrived,
// it is theirs — and has no gate on it, because mail from somebody you have
// never met is still mail you were sent. Conflating the two is how the inbox
// came to hold only the conversations you had started: nothing but
// agent-addressed mail was ever handed on, so nothing else was ever recorded,
// and a page whose whole claim is that things turn up in it showed an empty
// list to an account with a full mailbox.
//
// Spam is the one exclusion and it is made here rather than by each subscriber:
// it was refused at the door in every sense that matters, and a record full of
// it is not a record of anything.
func deliverInbound(m InboundMail, r wakeRequest) {
	if r.IsSpam {
		return
	}
	announce(event.EventMailReceived, m)
	if mayDispatch(r) {
		announce(event.EventMailForAgent, m)
	}
}

// announce puts a whole message on a topic.
//
// The message travels as JSON under one key rather than as a bag of strings,
// so a subscriber decodes it back into the type it was sent as. The bus carries
// map[string]interface{}, and spreading fifteen fields across it means every
// subscriber re-derives the struct by hand and one of them eventually spells a
// key differently.
func announce(topic string, m InboundMail) {
	b, err := json.Marshal(m)
	if err != nil {
		app.Log("mail", "could not announce %s: %v", topic, err)
		return
	}
	event.Publish(event.Event{Type: topic, Data: map[string]interface{}{
		"message": string(b),
	}})
}

// MessageFrom decodes what announce put on the bus.
//
// Exported because every subscriber needs it and hand-unpacking is the thing
// announce exists to prevent. Reports false when the payload is not a message,
// which is what a subscriber on the wrong topic gets.
func MessageFrom(data map[string]interface{}) (InboundMail, bool) {
	s, _ := data["message"].(string)
	if s == "" {
		return InboundMail{}, false
	}
	var m InboundMail
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return InboundMail{}, false
	}
	return m, true
}
