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
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
)

// WebhookHandler receives an inbound message from Twilio.
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validSignature(r, signedURLs(r), r.PostForm) {
		// Terse to the caller — anything more is a hint to whoever is probing —
		// and loud in the log, because the two reasons this happens look
		// identical from outside. Either somebody is poking at the endpoint, or
		// the URL Twilio signed is not the one reconstructed here, and the
		// second is a misconfiguration that silently drops every message.
		app.Log("sms", "webhook signature did not match. Tried %s. Set TWILIO_WEBHOOK_URL "+
			"to the address configured on the number if none of those is it",
			strings.Join(signedURLs(r), ", "))
		http.Error(w, "forbidden", http.StatusForbidden)
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
