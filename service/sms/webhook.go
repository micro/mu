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
	if !validSignature(r, publicURL(r), r.PostForm) {
		// Deliberately terse. Anything more is a hint to whoever is probing.
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

// publicURL rebuilds the address Twilio signed.
//
// The signature covers the URL as Twilio called it, which behind a proxy is not
// the URL this process sees: the scheme is https out there and http in here,
// and the host is the proxy's. MU_DOMAIN is the configured public name, so it
// is the authority when it is set.
func publicURL(r *http.Request) string {
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" && d != "localhost" {
		d = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://"), "/")
		return "https://" + d + r.URL.RequestURI()
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
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
