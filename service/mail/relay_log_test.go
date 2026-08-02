package mail

import (
	"fmt"
	"strings"
	"testing"
)

func resetRelayLog() {
	relayMu.Lock()
	relayLog = nil
	relayMu.Unlock()
}

func rawMessage(subject, messageID string) []byte {
	return []byte(strings.Join([]string{
		"From: Mu Events <no-reply@micro.mu>",
		"To: someone@gmail.com",
		"Subject: " + subject,
		"Message-ID: " + messageID,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
		"",
	}, "\r\n"))
}

func TestRecordRelayCapturesHeaders(t *testing.T) {
	resetRelayLog()
	defer resetRelayLog()

	recordRelay("no-reply@micro.mu", "someone@gmail.com",
		rawMessage("Event: Check the app", "<123.no-reply@micro.mu>"), nil)

	entries := RecentRelays(10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Subject != "Event: Check the app" {
		t.Errorf("subject = %q", e.Subject)
	}
	if e.MessageID != "<123.no-reply@micro.mu>" {
		t.Errorf("message id = %q", e.MessageID)
	}
	if e.From != "no-reply@micro.mu" || e.To != "someone@gmail.com" {
		t.Errorf("addresses = %q -> %q", e.From, e.To)
	}
	if !e.OK || e.Error != "" {
		t.Errorf("expected success, got ok=%v err=%q", e.OK, e.Error)
	}

	sent, failed := RelayStats()
	if sent != 1 || failed != 0 {
		t.Errorf("stats = %d sent, %d failed", sent, failed)
	}
}

func TestRecordRelayDecodesEncodedSubject(t *testing.T) {
	resetRelayLog()
	defer resetRelayLog()

	recordRelay("a@micro.mu", "b@example.com",
		rawMessage("=?utf-8?B?SGVsbG8gV29ybGQ=?=", "<1@micro.mu>"), nil)

	if got := RecentRelays(1)[0].Subject; got != "Hello World" {
		t.Errorf("subject = %q, want %q", got, "Hello World")
	}
}

func TestRecordRelayRecordsFailure(t *testing.T) {
	resetRelayLog()
	defer resetRelayLog()

	recordRelay("a@micro.mu", "b@example.com",
		rawMessage("Nope", "<2@micro.mu>"), fmt.Errorf("no MX for example.com"))

	e := RecentRelays(1)[0]
	if e.OK {
		t.Error("expected failure entry")
	}
	if !strings.Contains(e.Error, "no MX") {
		t.Errorf("error = %q", e.Error)
	}
	if sent, failed := RelayStats(); sent != 0 || failed != 1 {
		t.Errorf("stats = %d sent, %d failed", sent, failed)
	}
}

func TestRecordRelayUnparseableMessage(t *testing.T) {
	resetRelayLog()
	defer resetRelayLog()

	recordRelay("a@micro.mu", "b@example.com", []byte("not a message"), nil)

	e := RecentRelays(1)[0]
	if e.Subject != "(no subject)" {
		t.Errorf("subject = %q", e.Subject)
	}
	if e.To != "b@example.com" {
		t.Errorf("recipient should still be recorded, got %q", e.To)
	}
}

func TestRelayLogIsCappedNewestFirst(t *testing.T) {
	resetRelayLog()
	defer resetRelayLog()

	for i := 0; i < relayLogMax+20; i++ {
		recordRelay("a@micro.mu", "b@example.com",
			rawMessage(fmt.Sprintf("msg %d", i), fmt.Sprintf("<%d@micro.mu>", i)), nil)
	}

	entries := RecentRelays(0)
	if len(entries) != relayLogMax {
		t.Fatalf("log length = %d, want %d", len(entries), relayLogMax)
	}
	want := fmt.Sprintf("msg %d", relayLogMax+19)
	if entries[0].Subject != want {
		t.Errorf("newest entry = %q, want %q", entries[0].Subject, want)
	}
}
