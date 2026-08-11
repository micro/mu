package sms

import "testing"

// An API key sends and cannot receive, and nothing said so.
//
// Twilio accepts an API key (SK…) as a sending credential, so putting one in
// TWILIO_ACCOUNT_SID works and looks configured. A webhook signature is made
// with the account's own auth token, so with a key in that slot there is
// nothing to check a signature against — outbound fine, every reply refused,
// and no obvious connection between the two. It took a provider error log to
// find, which is the wrong place to keep something this checkable.
func TestAnAPIKeyIsNotAnAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "SK00000000000000000000000000000000")
	t.Setenv("TWILIO_AUTH_TOKEN", "the-key-secret")
	t.Setenv("TWILIO_FROM", "+447700900000")

	if AccountSID() != "" {
		t.Error("an API key was accepted as an account")
	}
	if CanReceive() == "" {
		t.Error("an instance that cannot verify a signature says it can receive")
	}
	// It can still send, because it could a minute ago and refusing would be a
	// second outage on top of the first.
	if !Configured() {
		t.Error("a working sending setup was declared unconfigured")
	}
	if user, pass := credentials(); user != "SK00000000000000000000000000000000" || pass != "the-key-secret" {
		t.Errorf("credentials = %q %q, want the key read as a key", user, pass)
	}
}

// Configured properly: an account, its auth token, and optionally a key to send
// with. The signature is always checked against the account's token.
func TestAccountAndKeyTogether(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	t.Setenv("TWILIO_AUTH_TOKEN", "the-account-auth-token")
	t.Setenv("TWILIO_FROM", "+447700900000")

	if CanReceive() != "" {
		t.Errorf("a correctly configured instance reports a problem: %s", CanReceive())
	}
	if authToken() != "the-account-auth-token" {
		t.Error("the signature would not be checked against the account's auth token")
	}
	if user, _ := credentials(); user != "AC00000000000000000000000000000000" {
		t.Error("without an API key, sending should use the account")
	}

	t.Setenv("TWILIO_API_KEY", "SKkey")
	t.Setenv("TWILIO_API_SECRET", "secret")
	if user, pass := credentials(); user != "SKkey" || pass != "secret" {
		t.Error("an API key should be preferred for sending")
	}
	if authToken() != "the-account-auth-token" {
		t.Error("an API key must not become the thing signatures are checked against")
	}
}
