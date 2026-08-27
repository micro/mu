package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
		"/stripe/webhook",
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

// A webhook must be reachable with no session, and Stripe's is the one that
// nearly was not.
//
// The path used to sit under whichever noun currently owned money. That made it
// a hostage to our own refactoring: /account is authenticated by prefix, so
// moving the webhook there would have made every top-up bounce with a 401 that
// Stripe reports as a delivery failure and this side never logs. Named for the
// provider at the top level, it is in nobody's prefix and stays public.
func TestTheStripeWebhookNeedsNoSession(t *testing.T) {
	authed := authRequired()
	for _, path := range []string{"/stripe/webhook"} {
		for prefix, needsAuth := range authed {
			if strings.HasPrefix(path, prefix) && needsAuth {
				t.Errorf("%s is behind auth via the %q prefix — Stripe has no session, "+
					"so every top-up would bounce", path, prefix)
			}
		}
	}
}

// The webhook is registered, and never as a redirect. Stripe POSTs, and a 303
// drops the body — the payment with it.
func TestTheStripeWebhookIsRegisteredDirectly(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `http.HandleFunc("/stripe/webhook", account.HandleStripeWebhook)`
	if !strings.Contains(string(src), want) {
		t.Errorf("missing %s — a webhook path that stops answering is a top-up "+
			"that is charged and never credited", want)
	}
	// And at the top level, under none of the prefixes money has lived under.
	//
	// This used to check the webhook was absent from a list of paths that
	// redirected; that list is gone, and the property it protected is not. Every
	// one of these prefixes redirects or 404s what it does not recognise, and
	// both answers lose a payment that has already been taken: a 303 on a POST
	// drops the body, and a 404 makes Stripe retry until it gives up.
	//
	// The path is named for the provider precisely so it never moves with our
	// rearrangements — it has been through three now.
	for _, prefix := range []string{"/account/", "/billing/", "/wallet/"} {
		if strings.HasPrefix("/stripe/webhook", prefix) {
			t.Errorf("the Stripe webhook sits under %s, which does not serve "+
				"unknown paths — a top-up would be charged and never credited", prefix)
		}
	}
}

// The gate verifies; the writer settles. Never the other way round.
//
// The whole fix is an ordering, and an ordering is exactly the kind of thing
// that gets undone by somebody tidying an import or inlining a call. So it is
// pinned where it lives: the request path must not settle, and the response
// must not be able to leave without the settle decision having been made.
func TestTheGateVerifiesAndDoesNotSettle(t *testing.T) {
	hooks, err := os.ReadFile("hooks.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hooks), "x402.VerifyAndSettle(") {
		t.Error("the request gate settles a payment before the tool has run, so a " +
			"call that fails afterwards is a charge with no answer — verify here " +
			"and let x402.Finish settle once there is something to hand back")
	}
	if !strings.Contains(string(hooks), "x402.Verify(") {
		t.Error("the request gate no longer verifies the payment at all")
	}

	serve, err := os.ReadFile("serve.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(serve)
	if !strings.Contains(body, "defer x402.Finish(w)") {
		t.Error("nothing settles the payment after the handler returns — the writer " +
			"holds the whole response back until Finish, so without it no paid " +
			"request answers at all")
	}
	// And it is deferred before the dispatch it guards, so a panic or an early
	// return still releases the response.
	//
	// LastIndex, not Index: there are two mux dispatches and the first is the
	// static-asset fast path, which returns long before any of this. Comparing
	// against that one said the defer was in the wrong place while it was in
	// the right one.
	if i, j := strings.Index(body, "defer x402.Finish(w)"),
		strings.LastIndex(body, "http.DefaultServeMux.ServeHTTP(w, r)"); i < 0 || j < 0 || i > j {
		t.Error("Finish is not deferred before the handler runs, so an early return " +
			"leaves the caller with nothing")
	}
}
