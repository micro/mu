package sms

// Every text goes in the record, whoever sent it.
//
// The same split agent/mail makes, and for the same reason its own comment
// gives: answering is a question about trust and about cost, recording is a
// question about fact. SMS only ever had the first, so the record of a
// conversation existed as a side effect of an agent replying to it — which
// meant the one thing a stranger must not be able to do was also the only thing
// that could file what they said. A text from a number nobody here knew was
// dropped with a log line.
//
// Now it is recorded and held. Held is the third state the record grew for
// this: in, visible, searchable, and acted on by nothing until somebody or
// agent/gate lets it through. See internal/thread/gate.go.
//
// This runs independently of answer() and either may go first. thread.Open is
// keyed on the number, so both find the same conversation and neither creates a
// second one.

import (
	"mu/agent"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/thread"
)

// LoadRecord subscribes to every text arriving.
func LoadRecord() {
	sub := event.Subscribe(event.SMSReceived)
	go func() {
		for e := range sub.Chan {
			t, ok := textedIn(e.Data)
			if !ok {
				app.Log("sms", "%s carried no message", event.SMSReceived)
				continue
			}
			known, _ := e.Data["known"].(bool)
			record(t, known)
		}
	}()
}

// record files an arriving text, holding it if the sender is unknown.
func record(t texted, known bool) {
	th := thread.Open(t.Owner, Client, t.From)
	if th == nil {
		return
	}
	// The number is the name of the conversation, because a phone has no
	// subject line and the person on the other end is the thread. Only on a new
	// one — thread.Name does nothing to a conversation that already has a name.
	thread.Name(t.Owner, th.ID, t.From)

	// Held before the words go in, not after.
	//
	// Between the two there is a conversation in the record that is not held,
	// and List is what the inbox reads: a stranger's text would appear in the
	// inbox and then vanish from it. Whether anybody saw it is a question about
	// timing, which is the wrong kind of question for this to depend on.
	if !known && !th.Held {
		thread.Hold(t.Owner, th.ID)
	}

	agent.SaidTo(t.Owner, th.ID, t.Text, "", t.From, "")

	// And say so once, for whoever judges these.
	//
	// After the message is in, so a subscriber that reads the conversation sees
	// what it is being asked about. Only for a held one: an arrival from
	// somebody known is not a question.
	if !known {
		event.Publish(event.Event{
			Type: event.ArrivalHeld,
			Data: map[string]interface{}{
				"account": t.Owner,
				"thread":  th.ID,
				"client":  Client,
				"from":    t.From,
			},
		})
	}
}
