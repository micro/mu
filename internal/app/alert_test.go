package app

// An alert nobody can find is not an alert.
//
// Three places in this codebase were writing the word CRITICAL at the front of
// an ordinary log line: the key store refusing a write that would have
// destroyed private keys, a settlement failing after the work had already been
// done, and the Stripe webhook secret being unset so every top-up notification
// was bounced. All three went into the same ring of five hundred lines as every
// routine message, which on a busy instance is an hour or two.
//
// A convention in a string is something nothing can count, filter or show you.

import (
	"strings"
	"testing"
)

func resetAlerts(t *testing.T) {
	t.Helper()
	sysLogMu.Lock()
	prevA, prevS := alertEntries, sysLogEntries
	alertEntries, sysLogEntries = nil, nil
	sysLogMu.Unlock()
	t.Cleanup(func() {
		sysLogMu.Lock()
		alertEntries, sysLogEntries = prevA, prevS
		sysLogMu.Unlock()
	})
}

func TestAnAlertIsKeptApartFromTheLog(t *testing.T) {
	resetAlerts(t)

	Alert("wallet", "refusing to write the key store: it would lose %d keys", 3)

	got := Alerts()
	if len(got) != 1 {
		t.Fatalf("%d alerts, want 1", len(got))
	}
	if !strings.Contains(got[0].Message, "lose 3 keys") {
		t.Errorf("the alert did not format its arguments: %q", got[0].Message)
	}
	if !got[0].Alert {
		t.Error("the entry is not marked as an alert")
	}
	if AlertCount() != 1 {
		t.Errorf("AlertCount = %d, want 1", AlertCount())
	}

	// It is in the ordinary log too, and only once: an admin reading the log in
	// order should see it in its place, but a duplicate would make the same
	// event look like two.
	n := 0
	for _, e := range SysLog() {
		if strings.Contains(e.Message, "lose 3 keys") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the alert appears %d times in the system log, want 1", n)
	}
}

// The ordinary log rolling over must not take the alerts with it. That is the
// whole reason they are held separately.
func TestAlertsSurviveTheLogRollingOver(t *testing.T) {
	resetAlerts(t)

	Alert("wallet", "the key store refused a write")
	for i := 0; i < sysLogMaxEntries+50; i++ {
		Log("news", "fetched a feed")
	}

	if got := AlertCount(); got != 1 {
		t.Fatalf("%d alerts after the log rolled, want 1", got)
	}
	if !strings.Contains(Alerts()[0].Message, "refused a write") {
		t.Error("the alert was lost to ordinary chatter, which is the thing this fixes")
	}
	// And the ordinary log really did roll, so the test proved something.
	for _, e := range SysLog() {
		if strings.Contains(e.Message, "refused a write") {
			t.Fatal("the log did not roll over — this test is not testing what it claims")
		}
	}
}

// Ordinary logging raises nothing. An alert that fires for routine work is one
// nobody reads.
func TestAnOrdinaryLogIsNotAnAlert(t *testing.T) {
	resetAlerts(t)
	Log("news", "fetched 30 headlines")
	if AlertCount() != 0 {
		t.Error("an ordinary log line raised an alert")
	}
}

// Alerts are bounded too. An instance alerting in a loop must not grow without
// limit.
func TestAlertsAreBounded(t *testing.T) {
	resetAlerts(t)
	for i := 0; i < alertMaxEntries+25; i++ {
		Alert("x402", "settlement failed")
	}
	if got := AlertCount(); got != alertMaxEntries {
		t.Errorf("held %d alerts, want the cap of %d", got, alertMaxEntries)
	}
}
