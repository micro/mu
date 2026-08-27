package sms

// What this service asks of the provider, and nothing the provider knows about
// texts. internal/twilio holds the credentials, the POST and the signature
// check; whether a message may be sent, to whom and at what price is here,
// because WhatsApp answers those differently and shares every line of the rest.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/twilio"
)

// send hands one message to the provider and returns its id.
//
// A variable so a test can stand in front of it. Everything interesting about
// this service is what happens either side of the provider call — the refusals
// before it and the bookkeeping after — and none of that was reachable by a
// test while the only way past this line was a live account.
var send = deliver

// sendOn is the provider call for one channel, and the seam a test stands in
// front of. It keeps send as the name of the ordinary case so every existing
// stub still works.
func sendOn(c Channel, to, body string) (string, error) {
	if c == ChannelSMS {
		return send(to, body)
	}
	return deliverOn(c, to, body)
}

// deliver is the real thing.
func deliver(to, body string) (string, error) { return deliverOn(ChannelSMS, to, body) }

// sendForm is the request this instance would make to send one message.
//
// Split out from the call so it can be looked at. Everything channel-specific
// about sending is in here — the wire prefix, which sender, whether a Messaging
// Service applies — and none of it was reachable by a test while it only
// existed in the moment before an HTTP request nobody could intercept. A
// WhatsApp message sent without its prefix is delivered as a text, from a
// number the recipient does not recognise, charged as a text.
func sendForm(c Channel, to, body string) (url.Values, error) {
	form := url.Values{"To": {c.wire(to)}, "Body": {body}}

	if c == ChannelWhatsApp {
		// One sender, not a pool. A WhatsApp sender is a number registered with
		// Meta against a business; a Messaging Service routing by country is
		// the wrong mechanism and would send from a number that is not
		// registered for WhatsApp at all.
		from := SendersFor(ChannelWhatsApp)
		if len(from) == 0 {
			return nil, fmt.Errorf("this instance has no WhatsApp sender — an operator sets TWILIO_WHATSAPP_FROM")
		}
		form.Set("From", c.wire(from[0]))
		return form, nil
	}

	if svc := messagingService(); svc != "" {
		// The sender pool picks the number, which is what it is for: with
		// Geomatch on it sends from the one in the handset's own country, and
		// it knows which of them are registered for what.
		form.Set("MessagingServiceSid", svc)
	} else if from := FromFor(to); from != "" {
		form.Set("From", from)
	} else {
		return nil, fmt.Errorf("no number is configured for that country — this instance sends from %s",
			strings.Join(Senders(), ", "))
	}
	return form, nil
}

// deliverOn hands one message to the provider on a channel.
//
// The wire prefix goes on here and nowhere else. Everywhere above this line a
// number is a number — internal/phone.Normalise refuses a prefixed one outright
// since a "whatsapp:+44…" sender was silently turned into a stranger's number.
func deliverOn(c Channel, to, body string) (string, error) {
	form, err := sendForm(c, to, body)
	if err != nil {
		return "", err
	}

	res, err := twilio.Send(form)
	if err != nil {
		// 21610 is Twilio's own opt-out list saying no. A Messaging Service
		// with Advanced Opt-Out answers STOP itself, so that message never
		// reaches our webhook and our list never learns — this is where it
		// finds out, and after this the refusal costs nothing.
		if res.Code == 21610 {
			OptOut(to)
		}
		return "", err
	}
	return res.SID, nil
}

// The provider's answers, under this package's names, so the rest of the
// service reads as it did before the provider moved out.
func AccountSID() string                  { return twilio.AccountSID() }
func pathSID() string                     { return twilio.PathSID() }
func authToken() string                   { return twilio.AuthToken() }
func credentials() (string, string)       { return twilio.Credentials() }
func signedURLs(r *http.Request) []string { return twilio.SignedURLs(r) }
func verifyInbound() bool                 { return twilio.VerifiesInbound() }
func looksLikeAuthToken(t string) bool    { return twilio.LooksLikeAuthToken(t) }

func validSignature(r *http.Request, candidates []string, form url.Values) bool {
	return twilio.ValidSignature(r, candidates, form)
}
