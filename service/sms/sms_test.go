package sms

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/service/contacts"
)

// A text cannot be unsent, is charged either way, and enough of them get the
// number blocked by carriers for everybody on the instance. So the rules that
// refuse are the service, and these are them.

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "AC-test")
	t.Setenv("TWILIO_AUTH_TOKEN", "token-test")
	t.Setenv("TWILIO_FROM", "+15550000000")
}

func TestNumbersAreOneNumberHoweverTheyAreWritten(t *testing.T) {
	setup(t)
	for _, in := range []string{"+1 (555) 010-9999", "+15550109999", "+1-555-010-9999", " +1.555.010.9999 "} {
		if got := e164(in); got != "+15550109999" {
			t.Errorf("e164(%q) = %q", in, got)
		}
	}
	// No country code is not a number, because guessing one is how you text a
	// stranger in a country you did not mean.
	if got := e164("07700900123"); got != "" {
		t.Errorf("a number with no country code should be refused, got %q", got)
	}
	t.Setenv("SMS_DEFAULT_COUNTRY", "44")
	if got := e164("07700900123"); got != "+447700900123" {
		t.Errorf("with a default country set, e164 = %q", got)
	}
}

// The allowlist is the only control that bounds the loss on a premium
// destination, because no per-message price is high enough for all of them.
func TestOnlyAllowedCountries(t *testing.T) {
	setup(t)
	if !countryAllowed("+447700900123") || !countryAllowed("+15550109999") {
		t.Error("the default list should allow the UK and the US")
	}
	if countryAllowed("+8801700000000") {
		t.Error("a country outside the list was allowed")
	}
	t.Setenv("SMS_COUNTRIES", "880")
	if !countryAllowed("+8801700000000") || countryAllowed("+447700900123") {
		t.Error("the operator's list is not being read")
	}
}

// Texting a stranger is the whole of the abuse, so knowing somebody has to come
// from something that already happened.
func TestYouCanOnlyTextANumberYouKnow(t *testing.T) {
	setup(t)
	const me = "acct-1"

	if Known(me, "+447700900123") {
		t.Fatal("an unknown number is known")
	}

	if _, err := contacts.Add(me, "Sam", "", "+44 7700 900123", ""); err != nil {
		t.Fatal(err)
	}
	if !Known(me, "+447700900123") {
		t.Error("somebody in the address book is not known")
	}
	// And the contact belongs to one account, not to the instance.
	if Known("acct-2", "+447700900123") {
		t.Error("one account's contact makes another account's stranger textable")
	}

	// A number that texted us first is a conversation, so a reply is fine.
	Record("acct-2", "in", "+447700900999", "hello", 1)
	if !Known("acct-2", "+447700900999") {
		t.Error("cannot reply to somebody who texted first")
	}
	if Known(me, "+447700900999") {
		t.Error("somebody else's inbound message made this number textable")
	}
}

// STOP belongs to the number, not to whichever account it was talking to, and
// nothing an account does clears it.
func TestStopIsHonoured(t *testing.T) {
	setup(t)
	const me = "acct-1"
	if _, err := contacts.Add(me, "Sam", "", "+447700900123", ""); err != nil {
		t.Fatal(err)
	}

	OptOut("+44 7700 900123")
	if !OptedOut("+447700900123") {
		t.Fatal("STOP was not recorded")
	}
	if _, err := Send(me, "+447700900123", "hello"); err == nil ||
		!strings.Contains(err.Error(), "asked not to receive") {
		t.Errorf("sending to a number that said STOP: %v", err)
	}

	OptIn("+447700900123")
	if OptedOut("+447700900123") {
		t.Error("START did not undo the opt-out")
	}
}

// Billed per segment, so counted per segment. One emoji in a long message
// halves the segment size and triples the price.
func TestSegments(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"short", 1},
		{strings.Repeat("a", 160), 1},
		{strings.Repeat("a", 161), 2},
		{strings.Repeat("a", 306), 2},
		{strings.Repeat("a", 307), 3},
		{"👍", 1},
		{strings.Repeat("a", 70) + "👍", 2},
	}
	for _, c := range cases {
		if got := Segments(c.text); got != c.want {
			t.Errorf("Segments(%d chars) = %d, want %d", len([]rune(c.text)), got, c.want)
		}
	}
}

func TestSendRefusesBeforeItCosts(t *testing.T) {
	setup(t)
	const me = "acct-1"

	for _, c := range []struct{ name, to, text, want string }{
		{"not a number", "hello", "hi", "international format"},
		{"our own number", "+15550000000", "hi", "own number"},
		{"nothing to say", "+447700900123", "   ", "nothing to send"},
		{"a stranger", "+447700900123", "hi", "only text a number you already know"},
		{"a country we do not send to", "+8801700000000", "hi", "does not send to"},
	} {
		if _, err := Send(me, c.to, c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want something about %q", c.name, err, c.want)
		}
	}

	// Long enough to be several messages is refused rather than quietly costing
	// several times what it looks like.
	if _, err := contacts.Add(me, "Sam", "", "+447700900123", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Send(me, "+447700900123", strings.Repeat("a", maxBody+1)); err == nil {
		t.Error("an over-long message was accepted")
	}
}

// The webhook writes into people's history and honours STOP, so an unsigned
// request must not reach either.
func TestWebhookRefusesUnsignedRequests(t *testing.T) {
	setup(t)

	form := url.Values{"From": {"+447700900123"}, "Body": {"STOP"}}
	r := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	WebhookHandler(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("unsigned webhook: status %d, want 403", w.Code)
	}
	if OptedOut("+447700900123") {
		t.Error("an unsigned request opted a number out")
	}
}

// An inbound message goes to whoever last texted that number, and to nobody at
// all if this instance never did.
func TestInboundGoesToTheConversationItAnswers(t *testing.T) {
	setup(t)
	if OwnerOf("+447700900123") != "" {
		t.Error("a number nobody has texted belongs to somebody")
	}
	Record("acct-1", "out", "+447700900123", "hello", 1)
	if got := OwnerOf("+447700900123"); got != "acct-1" {
		t.Errorf("OwnerOf = %q, want acct-1", got)
	}
	Record("acct-2", "out", "+447700900123", "hello from me", 1)
	if got := OwnerOf("+447700900123"); got != "acct-2" {
		t.Errorf("OwnerOf after a later send = %q, want acct-2", got)
	}
}

// Deleting an account takes its messages with it, and leaves the opt-out list
// alone: STOP was said by the number, and closing an account is not that number
// changing its mind.
func TestDeletingAnAccountKeepsTheOptOutList(t *testing.T) {
	setup(t)
	Record("acct-1", "out", "+447700900123", "hello", 1)
	OptOut("+447700900999")

	DeleteAll("acct-1")

	if len(History("acct-1", 10)) != 0 {
		t.Error("messages survived the account")
	}
	if !OptedOut("+447700900999") {
		t.Error("deleting an account cleared somebody else's opt-out")
	}
}

// And a correctly signed one is accepted — a signature check that always fails
// is indistinguishable from a broken inbound endpoint until somebody texts in.
func TestWebhookAcceptsASignedRequest(t *testing.T) {
	setup(t)
	t.Setenv("MU_DOMAIN", "example.com")
	Record("acct-1", "out", "+447700900123", "hello", 1)

	form := url.Values{"From": {"+447700900123"}, "Body": {"on my way"}}
	body := form.Encode()

	// Signed the way Twilio signs: the full public URL, then every field sorted
	// by name, HMAC-SHA1 under the auth token.
	mac := hmac.New(sha1.New, []byte("token-test"))
	mac.Write([]byte("https://example.com/sms/webhook" + "Body" + form.Get("Body") + "From" + form.Get("From")))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	r := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Twilio-Signature", sig)
	w := httptest.NewRecorder()

	WebhookHandler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("signed webhook: status %d", w.Code)
	}
	got := History("acct-1", 10)
	if len(got) == 0 || got[0].Text != "on my way" || got[0].Direction != "in" {
		t.Errorf("the message did not land: %+v", got)
	}
}
