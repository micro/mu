package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A provider posting to a webhook gets past CSRF, whichever provider it is.
//
// It did not. The exemption named /wallet/stripe/webhook exactly, so Twilio's
// inbound messages were refused with a 403 in middleware, before reaching a
// handler with a signature check waiting for them. From Twilio's side that is
// error 11200, "HTTP retrieval failure"; from this side it was nothing at all,
// because the request never became anybody's to log.
func TestWebhooksAreExemptFromCSRF(t *testing.T) {
	for _, path := range []string{
		"/wallet/stripe/webhook",
		"/sms/webhook",
		"/whatsapp/webhook",
	} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		if !csrfExempt(r) {
			t.Errorf("%s is not exempt — a provider has no session and no token to send", path)
		}
	}
}

// And an ordinary form post is not exempt, which is the whole point.
func TestOrdinaryPostsStillNeedAToken(t *testing.T) {
	for _, path := range []string{"/sms", "/blog", "/notes", "/account", "/webhooks"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		if csrfExempt(r) {
			t.Errorf("%s is exempt from CSRF and should not be", path)
		}
	}
}

// The credential cases: something carrying its own is not relying on a cookie,
// so there is nothing to forge.
func TestCredentialledRequestsAreExempt(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/blog", nil)
	r.Header.Set("Authorization", "Bearer something")
	if !csrfExempt(r) {
		t.Error("a bearer token is the credential and needs no CSRF token")
	}

	r = httptest.NewRequest(http.MethodPost, "/login", nil)
	if !csrfExempt(r) {
		t.Error("signing in has no session to carry a token from")
	}
}
