package whatsapp

// What arrives, at /whatsapp/twilio.
//
// A separate path from /whatsapp/webhook, which belongs to the Meta client and
// speaks a different protocol with a different signature. Two providers for one
// medium is a thing an instance may reasonably have — the client was there
// first — and giving them one endpoint would mean guessing which had called.

import (
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/twilio"
)

// WebhookHandler receives an inbound WhatsApp message from Twilio.
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	if twilio.VerifyInbound() {
		if !twilio.ValidSignature(r, twilio.SignedURLs(r), r.PostForm) {
			why := "signature did not match. tried: " + strings.Join(twilio.SignedURLs(r), " ")
			if r.Header.Get("X-Twilio-Signature") == "" {
				why = "no X-Twilio-Signature header on the request"
			} else if !twilio.LooksLikeAuthToken(twilio.AuthToken()) {
				why = "TWILIO_AUTH_TOKEN is not the account's auth token, so nothing can be verified"
			}
			app.Log("whatsapp", "webhook rejected: %s", why)
			http.Error(w, "forbidden: "+why, http.StatusForbidden)
			return
		}
	} else if to := number(r.PostForm.Get("To")); to == "" || to != From() {
		// Nothing to verify against, so correlate instead: a message claiming
		// to be for a number this instance does not answer on is not worth the
		// benefit of any doubt.
		app.Log("whatsapp", "unverified inbound refused: %q is not this instance's WhatsApp number", to)
		http.Error(w, "forbidden: not this instance's WhatsApp number", http.StatusForbidden)
		return
	}

	from := number(r.PostForm.Get("From"))
	body := strings.TrimSpace(r.PostForm.Get("Body"))
	if from == "" {
		blank(w)
		return
	}

	owner := OwnerOf(from)
	if owner == "" {
		// Nobody here has a claim on that number. Logged rather than stored: a
		// message from a stranger is not evidence about any account, and filing
		// it under one would be a way to put words in somebody's inbox.
		app.Log("whatsapp", "message from %s belongs to no account, dropped — "+
			"verify that number under /sms to claim it", from)
		blank(w)
		return
	}

	Record(owner, "in", from, body)
	blank(w)
}

// blank is a valid empty TwiML reply: received, nothing to say back.
func blank(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Response></Response>`))
}
