package server

// The security headers, and the one exception in them.
//
// Nothing checked these, so the directive that broke checkout was found by a
// person clicking Upgrade and reading a console message that pointed at a URL
// on this host while saying the form violated 'self'.

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func csp(t *testing.T) string {
	t.Helper()
	rr := httptest.NewRecorder()
	setSecurityHeaders(rr)
	got := rr.Header().Get("Content-Security-Policy")
	if got == "" {
		t.Fatal("no Content-Security-Policy is set at all")
	}
	return got
}

// A form may leave this origin for exactly one place.
//
// form-action is checked against where a form ends up, not only against its
// action attribute. Paying posts to /account/stripe/checkout — this host — and
// that handler answers 303 to a checkout URL Stripe has just minted, so 'self'
// alone blocks a POST it has already accepted. The failure is invisible from
// the server: the request arrives, the session is created, and the browser
// refuses to follow the redirect.
func TestAFormMayReachStripeCheckoutAndNowhereElseOffOrigin(t *testing.T) {
	directive := ""
	for _, part := range strings.Split(csp(t), ";") {
		if part = strings.TrimSpace(part); strings.HasPrefix(part, "form-action") {
			directive = part
		}
	}
	if directive == "" {
		t.Fatal("there is no form-action directive, so a form may be posted anywhere")
	}
	if !strings.Contains(directive, "'self'") {
		t.Errorf("form-action does not allow this origin: %q", directive)
	}
	if !strings.Contains(directive, "https://checkout.stripe.com") {
		t.Errorf("form-action does not allow Stripe Checkout, so paying is blocked "+
			"by the browser after the session has been created: %q", directive)
	}
	// Enumerated, not wildcarded. The point of this directive is to say where a
	// form may take somebody, and a wildcard says "anywhere Stripe ever hosts".
	if strings.Contains(directive, "*") {
		t.Errorf("form-action uses a wildcard: %q", directive)
	}
}

// The directives that must not quietly relax.
func TestTheHeadersThatKeepOtherPeoplesCodeOut(t *testing.T) {
	got := csp(t)
	for _, want := range []string{
		"default-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"frame-ancestors 'self'",
		"connect-src 'self'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from the policy: %s", want, got)
		}
	}

	rr := httptest.NewRecorder()
	setSecurityHeaders(rr)
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}
