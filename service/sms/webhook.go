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
	"mu/internal/settings"
)

// WebhookHandler receives an inbound message from Twilio.
// verifyInbound reports whether an arriving message must prove it is genuine.
//
// On by default, because unverified this endpoint lets anybody who knows the
// URL write into a person's message history and opt any number out of ever
// hearing from this instance. Off is a real choice with a real cost, and it
// belongs to whoever runs the instance rather than to whoever wrote this.
func verifyInbound() bool {
	v := strings.ToLower(strings.TrimSpace(settings.Get("SMS_VERIFY_INBOUND")))
	return v != "0" && v != "false" && v != "off" && v != "no"
}

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
		// Nobody here started this conversation. Logged, not stored: an
		// unsolicited message is not evidence about any account, and filing it
		// under one would be a way to put text in a stranger's inbox.
		app.Log("sms", "message from %s belongs to no conversation, dropped", from)
		twiml(w, "")
		return
	}

	Record(owner, "in", from, body, Segments(body))
	twiml(w, "")
}

// signedURLs is every address this request might have been signed as.
//
// The signature covers the URL as Twilio called it, and this process cannot see
// that: behind a proxy the scheme is https outside and http in here, and the
// host is the proxy's. Guessing once and rejecting on a miss is what turned a
// configuration detail into every inbound message vanishing, so guess several
// times and let the operator end the argument with TWILIO_WEBHOOK_URL.
func signedURLs(r *http.Request) []string {
	path := r.URL.RequestURI()

	var out []string
	add := func(u string) {
		for _, seen := range out {
			if seen == u {
				return
			}
		}
		out = append(out, u)
	}

	// What the operator says it is, which ends any disagreement.
	if u := strings.TrimSpace(settings.Get("TWILIO_WEBHOOK_URL")); u != "" {
		add(strings.TrimSuffix(u, "/"))
	}
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" && d != "localhost" {
		d = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://"), "/")
		add("https://" + d + path)
		add("http://" + d + path)
		add("https://www." + d + path)
	}
	if r.Host != "" {
		add("https://" + r.Host + path)
		add("http://" + r.Host + path)
	}
	return out
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
