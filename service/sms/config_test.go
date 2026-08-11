package sms

import (
	"strings"
	"testing"
)

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
	t.Setenv("TWILIO_AUTH_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("TWILIO_FROM", "+447700900000")

	if CanReceive() != "" {
		t.Errorf("a correctly configured instance reports a problem: %s", CanReceive())
	}
	if authToken() != "0123456789abcdef0123456789abcdef" {
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
	if authToken() != "0123456789abcdef0123456789abcdef" {
		t.Error("an API key must not become the thing signatures are checked against")
	}
}

// A key in the account slot cannot verify a signature and must still send.
//
// Reading that slot strictly for both questions stopped every outbound message
// on an instance that had been sending fine — the exact second outage the
// strictness was meant to prevent, caused by it. The two questions have two
// answers.
func TestAKeyInTheAccountSlotStillSends(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "SK00000000000000000000000000000000")
	t.Setenv("TWILIO_AUTH_TOKEN", "the-key-secret")
	t.Setenv("TWILIO_FROM", "+447700900000")

	if pathSID() != "SK00000000000000000000000000000000" {
		t.Errorf("pathSID = %q — the send URL needs whatever is configured", pathSID())
	}
	if AccountSID() != "" {
		t.Error("a key was read as an account, which is the question about signatures")
	}
	if !Configured() || CanReceive() == "" {
		t.Error("this setup sends and cannot receive, and should say exactly that")
	}
}

// The page said "no phone number configured" whatever was missing, which sent
// an operator to look at the number when the number was fine.
func TestMissingNamesWhatIsMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	t.Setenv("TWILIO_FROM", "+447700900000")
	t.Setenv("TWILIO_AUTH_TOKEN", "")

	got := Missing()
	if len(got) != 1 || !strings.Contains(got[0], "TWILIO_AUTH_TOKEN") {
		t.Errorf("Missing = %v, want just the auth token — the number is set", got)
	}

	t.Setenv("TWILIO_AUTH_TOKEN", "0123456789abcdef0123456789abcdef")
	if got := Missing(); len(got) != 0 {
		t.Errorf("Missing = %v on a complete setup", got)
	}

	t.Setenv("TWILIO_FROM", "")
	if got := Missing(); len(got) != 1 || !strings.Contains(got[0], "TWILIO_FROM") {
		t.Errorf("Missing = %v, want the number", got)
	}
}

// An account auth token is 32 hex characters. An API key secret is the same
// length and is not hex — indistinguishable by eye in a settings form, and the
// difference between receiving texts and refusing all of them.
func TestTheTokenShapeIsChecked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	t.Setenv("TWILIO_FROM", "+447700900000")

	t.Setenv("TWILIO_AUTH_TOKEN", "0123456789abcdef0123456789abcdef")
	if CanReceive() != "" {
		t.Errorf("a real-shaped auth token was rejected: %s", CanReceive())
	}

	t.Setenv("TWILIO_AUTH_TOKEN", "Zx9QwErTyUiOpAsDfGhJkLzXcVbNm012")
	if CanReceive() == "" {
		t.Error("an API-key-shaped secret was accepted as an auth token")
	}
}
