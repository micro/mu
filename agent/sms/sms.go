package sms

// Answering a text.
//
// The same seam as agent/mail and agent/chat: service/sms owns the number and
// may not call an agent, so it announces what arrived on event.SMSForAgent and
// this answers.
//
// The gate is upstream. service/sms publishes only for a sender the account
// knows — a verified number, or one this instance texted first — so nothing
// here has to decide whether a stranger deserves a model call.

import (
	"strings"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/thread"
	svcsms "mu/service/sms"
)

// Client is which door this was.
const Client = thread.SMSClient

// Load subscribes to texts that want an answer.
func Load() {
	sub := event.Subscribe(event.SMSForAgent)
	go func() {
		for e := range sub.Chan {
			t, ok := textedIn(e.Data)
			if !ok {
				app.Log("sms", "%s carried no message", event.SMSForAgent)
				continue
			}
			// One goroutine per text, recovered: answering is a model call and
			// several tool calls, and a panic on one must not stop the next.
			go func(t texted) {
				defer func() {
					if rec := recover(); rec != nil {
						app.Log("sms", "answering %s panicked: %v", t.From, rec)
					}
				}()
				answer(t)
			}(t)
		}
	}()
}

// texted is one message that is expecting an answer.
type texted struct {
	Owner string
	From  string
	Text  string
}

func textedIn(data map[string]interface{}) (texted, bool) {
	str := func(k string) string {
		v, _ := data[k].(string)
		return v
	}
	t := texted{Owner: str("owner"), From: str("from"), Text: str("text")}
	if t.Owner == "" || t.From == "" || strings.TrimSpace(t.Text) == "" {
		return texted{}, false
	}
	return t, true
}

// answer asks, then texts back.
func answer(t texted) {
	res, err := agent.Ask(agent.AskRequest{
		Account: t.Owner,
		Client:  Client,
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
	if len(reply) > replyLimit {
		reply = strings.TrimSpace(reply[:replyLimit-1]) + "…"
	}
	if _, err := svcsms.Send(t.Owner, t.From, reply); err != nil {
		app.Log("sms", "could not text %s back: %v", t.From, err)
	}
}

// replyLimit is how much of an answer fits in a text.
//
// Under service/sms's own maximum, because that one is a refusal and this is a
// trim: an answer that arrives shortened is worth more than one that does not
// arrive. Somebody who wants the whole thing has the same conversation waiting
// in their inbox, which is the point of one record across every door.
const replyLimit = 460
