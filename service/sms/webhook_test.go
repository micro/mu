package sms

import (
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
