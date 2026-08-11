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
		// benefit of any doubt. Both numbers named, because "not this
		// instance's number" without saying which one is expected is a sentence
		// nobody can act on — and the answer is usually that
		// TWILIO_WHATSAPP_FROM is unset.
		why := "addressed to " + to + ", and this instance answers WhatsApp on " + From()
		if From() == "" {
			why = "TWILIO_WHATSAPP_FROM is not set, so this instance has no WhatsApp number to be addressed on"
		}
		app.Log("whatsapp", "unverified inbound refused: %s", why)
		http.Error(w, "forbidden: "+why, http.StatusForbidden)
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
		// Nobody here has a claim on that number, so it goes nowhere: a message
		// from a stranger is not evidence about any account, and filing it under
		// one would be a way to put words in somebody's inbox.
		//
		// Said out loud in the response as well as the log. Two hundred and a
		// silent drop is indistinguishable from delivery at the provider's end,
		// and a message that vanishes without anybody being told is the worst
		// of the outcomes available. Still a 200, because it is not the
		// provider's fault and a retry would change nothing.
		why := from + " is not linked to any account here — verify that number at /sms to claim it"
		app.Log("whatsapp", "inbound dropped: %s", why)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("accepted, not delivered: " + why + "\n"))
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
