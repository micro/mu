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

// deliver is the real thing.
func deliver(to, body string) (string, error) {
	form := url.Values{"To": {to}, "Body": {body}}
	if svc := messagingService(); svc != "" {
		// The sender pool picks the number, which is what it is for: with
		// Geomatch on it sends from the one in the handset's own country, and
		// it knows which of them are registered for what.
		form.Set("MessagingServiceSid", svc)
	} else if from := FromFor(to); from != "" {
		form.Set("From", from)
	} else {
		return "", fmt.Errorf("no number is configured for that country — this instance sends from %s",
			strings.Join(Senders(), ", "))
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
func verifyInbound() bool                 { return twilio.VerifyInbound() }
func looksLikeAuthToken(t string) bool    { return twilio.LooksLikeAuthToken(t) }

func validSignature(r *http.Request, candidates []string, form url.Values) bool {
	return twilio.ValidSignature(r, candidates, form)
}
