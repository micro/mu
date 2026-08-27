package sms

// What arrives.
//
// One number serves the whole instance, so an inbound message has to be given
// to somebody: it goes to the account that most recently texted that number,
// which is the conversation it is a reply to. A message from a number nobody
// here has texted belongs to nobody, and is dropped rather than handed to
// whoever happens to be first in a list.
//
// Two things have to be right or this endpoint is a liability. It must verify
// the request really came from the provider — it is a public URL that writes
// into people's message history, and it honours STOP, so an unauthenticated
// version lets a stranger opt any number out of ever hearing from this instance
// again. And it must answer quickly with valid TwiML, because a provider that
// gets an error retries, and a retried STOP is harmless but a retried message
// is a duplicate in somebody's history.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/settings"
)

// WebhookHandler receives an inbound message from Twilio.
// implausible says why an unverified message does not add up, or "" if it does.
//
// Correlation rather than proof. The message names the account it came from and
// the number it was sent to, and this instance knows both — so a message for
// somebody else's number, or from an account this instance has nothing to do
// with, can be refused without any cryptography. It raises the bar from "anyone
// who knows this URL" to "anyone who knows this URL and our numbers and our
// account", which is not security but is not nothing, and it is what is
// available when there is no auth token to check a signature against.
func implausible(r *http.Request) string {
	to := e164(r.PostForm.Get("To"))
	if to == "" {
		return "no To on the message"
	}
	if !Ours(to) {
		return to + " is not a number this instance sends from"
	}
	if want := AccountSID(); want != "" {
		if got := r.PostForm.Get("AccountSid"); got != "" && got != want {
			return "account " + got + " is not this instance's account"
		}
	}
	if want := messagingService(); want != "" {
		if got := r.PostForm.Get("MessagingServiceSid"); got != "" && got != want {
			return "messaging service " + got + " is not this instance's"
		}
	}
	return ""
}

func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	// Verification is right and it is not worth losing every message over. An
	// instance authenticating with an API key has no account auth token, so
	// there is nothing a signature can be checked against — and rejecting is
	// then a choice to receive nothing at all, made on the operator's behalf
	// without asking. They can say otherwise.
	if !verifyInbound() {
		// No signature to check, so check what the message says about itself.
		// None of it is proof — every field is forgeable by whoever knows the
		// URL — but a message claiming to be for a number this instance does
		// not own, or from an account it does not use, is not worth the benefit
		// of any doubt.
		if why := implausible(r); why != "" {
			app.Log("sms", "unverified inbound message refused: %s", why)
			http.Error(w, "forbidden: "+why, http.StatusForbidden)
			return
		}
	} else if !validSignature(r, signedURLs(r), r.PostForm) {
		// Terse to the caller — anything more is a hint to whoever is probing —
		// and loud in the log, because the two reasons this happens look
		// identical from outside. Either somebody is poking at the endpoint, or
		// the URL Twilio signed is not the one reconstructed here, and the
		// second is a misconfiguration that silently drops every message.
		// The reason goes in the body as well as the log, because the log is
		// on a machine and the body is in the provider's own request
		// inspector — which is where somebody debugging a webhook already is.
		// Nothing here is a secret: these are this instance's own public
		// addresses, and a signature is not reversible without the token.
		// The parameter names matter as much as the URL: the signature covers
		// them, and a body that did not survive the proxy signs as nothing at
		// all while looking from here like a request that simply did not match.
		names := make([]string, 0, len(r.PostForm))
		for k := range r.PostForm {
			names = append(names, k)
		}
		sort.Strings(names)
		why := fmt.Sprintf("signature did not match. tried: %s. %d params: %s",
			strings.Join(signedURLs(r), " "), len(names), strings.Join(names, ","))
		if r.Header.Get("X-Twilio-Signature") == "" {
			why = "no X-Twilio-Signature header on the request"
		}
		if problem := CanReceive(); problem != "" {
			why = problem
		}
		// Checkable without holding any secret: Twilio signs with the auth token
		// of the account that owns the message, so a number on a different
		// account — a subaccount, or a token rotated since — cannot verify
		// however right the URL is. The request says which account it came from.
		if got := r.PostForm.Get("AccountSid"); got != "" && AccountSID() != "" && got != AccountSID() {
			why = "this message is from account " + got + " and TWILIO_ACCOUNT_SID is " +
				AccountSID() + " — the signature is made with the owning account's auth token"
		}
		app.Log("sms", "webhook rejected: %s", why)
		http.Error(w, "forbidden: "+why, http.StatusForbidden)
		return
	}

	from := e164(r.PostForm.Get("From"))
	body := strings.TrimSpace(r.PostForm.Get("Body"))
	if from == "" {
		twiml(w, "")
		return
	}

	// STOP first, and before anything is stored. Somebody asking to be left
	// alone should not have the message that says so filed under the account
	// they are trying to get away from.
	switch word := strings.ToLower(strings.TrimSpace(body)); {
	case stopWords[word]:
		OptOut(from)
		twiml(w, "You will not get any more messages from this number.")
		return
	case word == "start" || word == "unstop":
		OptIn(from)
		twiml(w, "You will get messages from this number again.")
		return
	case word == "help":
		twiml(w, "This number belongs to "+instanceName()+". Reply STOP to stop.")
		return
	}

	owner := OwnerOf(from)
	if owner == "" {
		// Nobody to file it under at all. OwnerOf falls back to the operator, so
		// this is an instance with no operator configured — there is no account
		// this could belong to, and inventing one is worse than losing it.
		app.Log("sms", "message from %s belongs to no account, dropped", from)
		twiml(w, "")
		return
	}

	Record(owner, "in", from, body, Segments(body))

	known, isKnown := KnownSender(from)

	// It arrived. Said with no gate on it, the way mail says MailReceived.
	//
	// This did not exist, and its absence is why an unsolicited text vanished:
	// the only path into the record was the side effect of an agent answering,
	// so the one thing a stranger must not be able to do was also the only thing
	// that could file what they said. A subscriber decides what to do with an
	// arrival from somebody unknown — agent/sms records it held — and that is a
	// judgement about trust, which is not this service's to make.
	event.Publish(event.Event{
		Type: event.SMSReceived,
		Data: map[string]interface{}{
			"owner": owner,
			"from":  from,
			"text":  body,
			"known": isKnown,
		},
	})

	// And wake an agent, for a sender the account knows.
	//
	// OwnerOf above falls back to the operator so nothing is lost, which is
	// right for filing and wrong for this: the fallback is a real account with
	// real credits, and any stranger who dialled the number would be talking to
	// their agent. KnownSender is the same lookup without that step — verified,
	// or a number this instance texted first, which are the two things a
	// stranger cannot arrange.
	//
	// Announced rather than answered here. A service does not call an agent;
	// agent/sms subscribes and replies through Send, which is where every rule
	// about what a text costs already lives.
	if isKnown {
		event.Publish(event.Event{
			Type: event.SMSForAgent,
			Data: map[string]interface{}{
				"owner": known,
				"from":  from,
				"text":  body,
			},
		})
	}

	// Empty, always. The reply goes out as its own message so it is charged,
	// recorded and capped like any other — a body in the TwiML response would
	// be a second send path that skipped all three.
	twiml(w, "")
}

func instanceName() string {
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" {
		return d
	}
	return "this service"
}

// twiml answers the provider. An empty reply is a valid one and means "nothing
// to say back".
func twiml(w http.ResponseWriter, reply string) {
	w.Header().Set("Content-Type", "application/xml")
	if reply == "" {
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Response></Response>`))
		return
	}
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Response><Message>` +
		html.EscapeString(reply) + `</Message></Response>`))
}
