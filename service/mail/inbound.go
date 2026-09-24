package mail

// The transport publishes acceptance and authentication facts. Subscribers
// decide whether to respond; the durable stored-message event independently
// maintains Inbox's view.

import (
	"encoding/json"

	"mu/internal/app"
	"mu/internal/event"
)

// Tagged is the address every account has a private version of: you+anything@.
// Kept as the name of that shape, which cc.go and the agent roster both read.
const Tagged = "+"

// arrival is transport metadata committed with the message. It carries no
// duplicate body; subscribers read the message through Server.Source.
type arrival struct {
	To            string   `json:"to"`
	Shared        bool     `json:"shared"`
	Authenticated bool     `json:"authenticated"`
	Owned         bool     `json:"owned"`
	Machine       bool     `json:"machine"`
	Others        []string `json:"others,omitempty"`
	ToAgent       bool     `json:"to_agent"`
	InReplyTo     string   `json:"in_reply_to,omitempty"`
	References    string   `json:"references,omitempty"`
}

// deliveredTo is the address a local delivery arrived at, which a Delivery
// carries as an account and a tag rather than as a string.
func deliveredTo(accountID, tag string) string {
	local := accountID
	if tag != "" {
		local += Tagged + tag
	}
	return EmailForUser(local, ConfiguredDomain())
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
