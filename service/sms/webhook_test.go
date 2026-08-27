package sms

import (
	"time"

	"mu/internal/auth"
	"mu/internal/event"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The signature covers the URL as Twilio called it, and this process cannot see
// that URL. Getting it wrong fails closed and silent: Twilio reports 11200,
// "HTTP retrieval failure", which reads like the server is down, and every
// inbound message is dropped.
func TestSignedURLCandidates(t *testing.T) {
	setup(t)
	t.Setenv("MU_DOMAIN", "micro.mu")

	r := httptest.NewRequest(http.MethodPost, "/sms/webhook", nil)
	r.Host = "10.0.0.4:8080" // what a proxy leaves behind

	got := signedURLs(r)
	want := map[string]bool{
		"https://micro.mu/sms/webhook":      false,
		"http://micro.mu/sms/webhook":       false,
		"https://www.micro.mu/sms/webhook":  false,
		"https://10.0.0.4:8080/sms/webhook": false,
	}
	for _, u := range got {
		if _, ok := want[u]; ok {
			want[u] = true
		}
	}
	for u, seen := range want {
		if !seen {
			t.Errorf("%s is not among the candidates: %v", u, got)
		}
	}

	// The operator's word settles it, and comes first.
	t.Setenv("TWILIO_WEBHOOK_URL", "https://sms.example.com/hook/")
	if first := signedURLs(r)[0]; first != "https://sms.example.com/hook" {
		t.Errorf("first candidate = %q, want the configured URL with its trailing slash trimmed", first)
	}
}

// Without a signature to check, correlate what the message says about itself.
//
// It is not proof — every field is forgeable by whoever knows the URL — but a
// message addressed to a number this instance does not own is not worth the
// benefit of any doubt, and refusing it costs nothing.
func TestUnverifiedMessagesAreStillCorrelated(t *testing.T) {
	setup(t)

	ours := func(extra map[string]string) *http.Request {
		form := url.Values{"To": {"+447700900000"}, "From": {"+447700900123"}, "Body": {"hi"}}
		for k, v := range extra {
			form.Set(k, v)
		}
		r := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ParseForm() //nolint:errcheck
		return r
	}

	if why := implausible(ours(nil)); why != "" {
		t.Errorf("a message to our own number was refused: %s", why)
	}
	if why := implausible(ours(map[string]string{"To": "+15550009999"})); why == "" {
		t.Error("a message addressed to somebody else's number was accepted")
	}

	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	if why := implausible(ours(map[string]string{"AccountSid": "ACsomebodyelse"})); why == "" {
		t.Error("a message from another account was accepted")
	}
	if why := implausible(ours(map[string]string{"AccountSid": "AC00000000000000000000000000000000"})); why != "" {
		t.Errorf("a message from our own account was refused: %s", why)
	}
}

// A stranger's text is announced as a fact and never as work.
//
// This is the invariant the whole gate rests on. Anybody can text a number that
// is printed on the internet, so if an unknown sender could publish
// SMSForAgent, anybody could run somebody else's agent on somebody else's
// credits by sending one message. SMSReceived carries the fact so it can be
// recorded and held; SMSForAgent is the permission and is not given.
func TestAStrangerIsAnnouncedButNeverHandedToAnAgent(t *testing.T) {
	setup(t)
	// The correlation path rather than the signature one. An instance
	// authenticating with an API key has no account auth token to check a
	// signature against, which is a real configuration and the one this covers
	// — the signature path has its own tests.
	t.Setenv("SMS_VERIFY_INBOUND", "off")
	// OwnerOf falls back to the operator, which is the oldest admin account —
	// so without one there is nobody to file a stranger's text under and the
	// drop this test is about would happen for a different reason.
	if err := auth.Create(&auth.Account{
		ID: "operator", Name: "operator", Admin: true, Created: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if auth.Operator() == "" {
		t.Fatal("no operator, so a stranger's text has nowhere to be filed")
	}

	got := event.Subscribe(event.SMSReceived)
	work := event.Subscribe(event.SMSForAgent)

	form := url.Values{
		"To": {"+447700900000"}, "From": {"+447700900555"},
		"Body": {"cheap watches, click here"},
	}
	r := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	WebhookHandler(httptest.NewRecorder(), r)

	select {
	case e := <-got.Chan:
		if known, _ := e.Data["known"].(bool); known {
			t.Error("a number nobody here has texted was reported as known")
		}
		if e.Data["text"] != "cheap watches, click here" {
			t.Errorf("the announcement lost the message: %v", e.Data["text"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a text from a stranger was not announced at all — it was dropped, " +
			"which is what this change exists to stop")
	}

	select {
	case <-work.Chan:
		t.Fatal("a stranger's text was handed to an agent. Anybody can text a " +
			"published number, so this is a way to spend somebody else's credits")
	case <-time.After(300 * time.Millisecond):
	}
}
