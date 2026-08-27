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
	// A US number and a UK one, which is what two countries takes.
	t.Setenv("TWILIO_FROM", "+15550000000,+447700900000")
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

// Known is no longer a condition of sending, but it still decides what the page
// offers and what SMS_KNOWN_ONLY restricts to, so it has to be right.
func TestKnown(t *testing.T) {
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
		{"our own US number", "+15550000000", "hi", "own number"},
		{"our own UK number", "+447700900000", "hi", "own number"},
		{"nothing to say", "+447700900123", "   ", "nothing to send"},
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
	// Nobody has a claim or a conversation, so it goes to whoever runs the
	// instance — a message nobody sees is worse than one somebody unexpected
	// does. With no admin there is nobody to give it to.
	if got := OwnerOf("+447700900123"); got != Fallback() {
		t.Errorf("OwnerOf = %q with no claim and no conversation, want the fallback %q", got, Fallback())
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

// One number does not serve two countries: a US long code texting a UK handset
// is filtered by UK carriers, and a UK number texting a US handset is blocked
// outright. So the sender is chosen by where the message is going, and a
// country with no number of its own is refused rather than sent from whichever
// number happened to be first.
func TestTheSenderMatchesTheDestination(t *testing.T) {
	setup(t)

	if got := FromFor("+447700900123"); got != "+447700900000" {
		t.Errorf("a UK destination should send from the UK number, got %q", got)
	}
	if got := FromFor("+15550109999"); got != "+15550000000" {
		t.Errorf("a US destination should send from the US number, got %q", got)
	}

	t.Setenv("TWILIO_FROM", "+15550000000")
	if got := FromFor("+447700900123"); got != "" {
		t.Errorf("with no UK number, a UK destination should have no sender, got %q", got)
	}

	const me = "acct-1"
	if _, err := contacts.Add(me, "Sam", "", "+447700900123", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Send(me, "+447700900123", "hi"); err == nil ||
		!strings.Contains(err.Error(), "no number in that country") {
		t.Errorf("sending where there is no local number: %v", err)
	}
}

// Both of the instance's own numbers are its own, not just the first.
func TestOurs(t *testing.T) {
	setup(t)
	for _, n := range []string{"+15550000000", "+44 7700 900000"} {
		if !Ours(n) {
			t.Errorf("%s is one of ours and was not recognised", n)
		}
	}
	if Ours("+447700900123") {
		t.Error("somebody else's number was taken for ours")
	}
}

// The rule that a caller may only text somebody they already know is off by
// default, because contacts_add takes any number and defeated it in one call.
// An operator can still ask for it.
func TestKnownOnlyIsOptIn(t *testing.T) {
	setup(t)
	if KnownOnly() {
		t.Error("sending is restricted to known numbers by default")
	}
	t.Setenv("SMS_KNOWN_ONLY", "true")
	if !KnownOnly() {
		t.Error("an operator asked for the restriction and did not get it")
	}
	if _, err := Send("acct-1", "+447700900123", "hi"); err == nil ||
		!strings.Contains(err.Error(), "numbers you already know") {
		t.Errorf("with the restriction on, a stranger should be refused: %v", err)
	}
}

// A loop is the failure that spends money without anybody deciding to. An agent
// that retries on a timeout sends the same sentence forty times, and every one
// of them is charged and delivered.
func TestTheSameMessageTwiceIsRefused(t *testing.T) {
	setup(t)
	const me, them = "acct-1", "+447700900123"

	Record(me, "out", them, "on my way", 1)
	if !Repeated(me, them, "on my way") {
		t.Fatal("a repeat was not recognised")
	}
	if _, err := Send(me, them, "on my way"); err == nil ||
		!strings.Contains(err.Error(), "a moment ago") {
		t.Errorf("resending the same message: %v", err)
	}
	// A different message to the same number is a conversation, not a loop.
	if Repeated(me, them, "actually, ten minutes") {
		t.Error("a different message was taken for a repeat")
	}
	// And the same message to somebody else is not a loop either.
	if Repeated(me, "+447700900999", "on my way") {
		t.Error("a different number was taken for a repeat")
	}
}

// Signing up is free and takes a minute, so a fresh account gets a much smaller
// cap — it is the only thing between a script and the daily allowance.
func TestNewAccountsGetASmallerAllowance(t *testing.T) {
	setup(t)
	if got := LimitFor("nobody-in-particular"); got != DailyLimit() {
		t.Errorf("LimitFor = %d, want the full %d for an established account", got, DailyLimit())
	}

	// Zero is the kill switch, and it is the same setting rather than a second
	// one, because an operator reaching for it is in a hurry.
	t.Setenv("SMS_DAILY_LIMIT", "0")
	if got := LimitFor("nobody-in-particular"); got != 0 {
		t.Errorf("LimitFor = %d with sending off, want 0", got)
	}
	if _, err := Send("acct-1", "+447700900123", "hi"); err == nil ||
		!strings.Contains(err.Error(), "not sending texts") {
		t.Errorf("with sending off: %v", err)
	}
}

// A number that is not a number is refused, not repaired.
//
// Normalise skipped whatever it did not recognise and kept the digits, which is
// right for the way people punctuate a number and catastrophic for anything
// else. Twilio labels a WhatsApp sender "whatsapp:+447700900123". The letters
// and the colon were dropped; the + was then no longer at the front so it went
// too; what was left had no country code, so the instance default was prepended
// and the answer was +44447700900123 — a real number, belonging to a stranger.
//
// The /whatsapp/twilio route has been pointing at this handler the whole time,
// so every WhatsApp message that ever arrived was filed against, and could have
// been replied to at, the wrong person's phone.
func TestSomethingThatIsNotANumberIsRefusedRatherThanRepaired(t *testing.T) {
	setup(t)
	t.Setenv("SMS_DEFAULT_COUNTRY", "44")

	// The case that was silently wrong.
	if got := e164("whatsapp:+447700900123"); got != "" {
		t.Errorf("e164(%q) = %q — a channel-prefixed sender was turned into a "+
			"number rather than refused", "whatsapp:+447700900123", got)
	}

	for _, in := range []string{
		"whatsapp:+447700900123",
		"sms:+447700900123",
		"MICROMU",             // an alphanumeric sender id is not a destination
		"+44 7700 900123 x22", // an extension is structure this cannot carry
		"+447700900123/+447700900124",
		"tel:+447700900123",
	} {
		if got := e164(in); got != "" {
			t.Errorf("e164(%q) = %q, want empty", in, got)
		}
	}

	// And the ways people really do write one still work.
	for in, want := range map[string]string{
		"+447700900123":      "+447700900123",
		"+44 7700 900 123":   "+447700900123",
		"+44 (7700) 900-123": "+447700900123",
		"07700900123":        "+447700900123",
		" +44.7700.900123 ":  "+447700900123",
	} {
		if got := e164(in); got != want {
			t.Errorf("e164(%q) = %q, want %q", in, got, want)
		}
	}
}
