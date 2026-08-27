// Package gate is the judgement about whether something that arrived from a
// stranger should be let in.
//
// # Why it is one agent and not one per channel
//
// A text from an unknown number, a federated chat from an unknown JID and mail
// from an address nobody here has written to are the same question asked three
// times: is this wanted, or is it somebody spraying a shared address. Building
// a judge into each service gets three prompts that drift apart and three places
// the answer to "what counts as spam here" lives.
//
// So the channels do their own deterministic job — record the arrival, hold it
// — and publish event.ArrivalHeld. This subscribes. Whatever grows a door next
// publishes the same fact and gets the same judgement for free.
//
// # It is never in the way
//
// The arrival is already in the record before this runs, and holding it is the
// channel's decision, not this one's. All this can do is release. So an
// instance with no model, or a provider that is down, is an instance where held
// arrivals sit held and visible until somebody looks — which is exactly what
// happens today with no judge at all, and is the safe direction to fail in.
//
// The opposite arrangement — judge first, record if allowed — would make a
// model outage into silently dropped messages, and dropped is the one outcome
// nobody can detect.
//
// # Reluctant on purpose
//
// The costly mistake is not letting a spammer through: that is one line in an
// inbox, next to a button that blocks the number. It is holding back a real
// message — the plumber texting to say he is outside — because held is quiet by
// design and nobody finds out. So the prompt releases when in doubt, and only
// the clear cases stay held.
package gate

import (
	"strings"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/thread"
)

// Load subscribes to arrivals that were held. Called at boot.
func Load() {
	go func() {
		sub := event.Subscribe(event.ArrivalHeld)
		for e := range sub.Chan {
			account, _ := e.Data["account"].(string)
			id, _ := e.Data["thread"].(string)
			from, _ := e.Data["from"].(string)
			if account == "" || id == "" {
				continue
			}
			// One goroutine per arrival: this is a model call and the
			// subscription channel is small and drops when it is full. Same
			// reason agent/moderate and agent/mail do it.
			go judge(account, id, from)
		}
	}()
}

// judge decides whether one held conversation should be let in.
func judge(account, id, from string) {
	defer func() {
		if rec := recover(); rec != nil {
			app.Log("gate", "judging %s panicked: %v", id, rec)
		}
	}()
	if !Configured() {
		return
	}

	said := arrival(account, id)
	if said == "" {
		return
	}

	verdict, err := ai.Ask(&ai.Prompt{
		System:   prompt,
		Question: "From: " + from + "\n\nMessage: " + said,
		Model:    ai.BackgroundModel(),
	})
	if err != nil {
		// Worth a line. A gatekeeper that has stopped working looks exactly
		// like one that is holding everything correctly, and the difference is
		// invisible from the inbox.
		app.Log("gate", "could not judge %s: %v", id, err)
		return
	}

	switch strings.ToUpper(strings.TrimSpace(verdict)) {
	case "HOLD":
		app.Log("gate", "held %s from %s", id, from)
	default:
		// Anything that is not a clear HOLD is let in, including a verdict this
		// does not recognise. A model that answers oddly must not become a
		// reason somebody's message disappears.
		thread.Release(account, id)
		app.Log("gate", "let in %s from %s", id, from)
	}
}

// arrival is what was said, which is what the judgement is about.
//
// The first message rather than the last: a held conversation has exactly one
// message when this runs, and if a second has landed in between it is the
// opening line that decides whether the sender should have been able to send
// the second at all.
func arrival(account, id string) string {
	msgs := thread.Messages(account, id, 1)
	if len(msgs) == 0 {
		return ""
	}
	said := strings.TrimSpace(msgs[0].Text)
	if len(said) > readLimit {
		said = said[:readLimit] + "…"
	}
	return said
}

// readLimit is how much of an arrival is worth reading to classify it. A text
// is 160 characters and a spam text is not subtler at the end than at the
// start.
const readLimit = 1000

// Configured reports whether this instance can judge an arrival at all.
//
// So a page can say "nothing is being judged, held messages are waiting for
// you" rather than showing a list that reads as a decision somebody made.
func Configured() bool { return ai.Configured() }

// prompt is what the model is asked.
//
// Deliberately one-sided. Releasing a spam message costs one line in an inbox
// beside a button that blocks the sender. Holding a real one costs a message
// somebody needed and will not know to look for — the plumber outside the
// house, the courier at the door, a number that changed. Those are not
// symmetrical and the prompt should not pretend they are.
const prompt = `You decide whether a message from an unknown sender should reach somebody's inbox.

This is a personal assistant. Unknown senders are usually legitimate: a tradesperson, a delivery driver, a doctor's surgery, a friend texting from a new number, a business replying to something the person started.

Answer with ONE WORD:
- HOLD — bulk marketing, a scam or phishing attempt, a one-time-password or verification code the person did not ask for, or abuse.
- LET_IN — everything else.

When in doubt, LET_IN. A held message is one the person never finds out about, and missing a real message matters far more than seeing one piece of spam.

Respond with just the single word.`
