package account

// The number, on the page where the other addresses are.

import (
	"strings"
	"testing"

	"mu/internal/settings"
	"mu/service/sms"
)

// Phone sits with Email because it is the same kind of fact.
//
// It lived inside a <details> on /sms headed "Verify a number as your own" —
// a service page, collapsed, where nobody looking for their account's contact
// details would open it. What made that wrong rather than merely tidy is what
// a verified number now decides: a text from one wakes your agent, and a text
// from any other number is filed and answered by nobody.
func TestThePhoneCardOffersVerificationLikeEmail(t *testing.T) {
	// Every var Configured() reads, so this runs rather than skips. A test
	// that skips when the thing it is about is switched off is a test that
	// stops running the day somebody changes what "switched off" means — and
	// eighteen of those were found in this repository today.
	for k, v := range map[string]string{
		"TWILIO_FROM":        "+447700900000",
		"TWILIO_ACCOUNT_SID": "ACnot-a-real-sid",
		"TWILIO_AUTH_TOKEN":  "not-a-real-auth-token-0000000000",
	} {
		prev := settings.Get(k)
		t.Cleanup(func() { settings.Set(k, prev) })
		settings.Set(k, v)
	}
	if !sms.Configured() {
		t.Fatal("sms is still not configured, so this test proves nothing — " +
			"Configured() reads something this test does not set")
	}

	got := renderPhoneCard("nobody_has_this_account")
	if got == "" {
		t.Fatal("no phone card on an instance that can text")
	}
	for _, want := range []string{"Phone", "verify_number", "/account"} {
		if !strings.Contains(got, want) {
			t.Errorf("the card does not contain %q:\n%s", want, got)
		}
	}
}

// And it is absent where it could only fail.
//
// The same call the wallet's convert form makes: a form that cannot work reads
// as broken rather than as unconfigured, and this page is where somebody goes
// when something is not working.
func TestNoPhoneCardWithoutANumberToTextFrom(t *testing.T) {
	prev := settings.Get("TWILIO_FROM")
	t.Cleanup(func() { settings.Set("TWILIO_FROM", prev) })
	settings.Set("TWILIO_FROM", "")

	if got := renderPhoneCard("nobody_has_this_account"); got != "" {
		t.Errorf("a phone card was drawn on an instance with no number:\n%s", got)
	}
}
