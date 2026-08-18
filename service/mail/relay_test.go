package mail

// Which of the two ways out is being used, and that it is a setting rather than
// a rebuild.

import (
	"os"
	"strings"
	"testing"
)

// source reads a file in this package, for the assertions that are about the
// code rather than about what it does — a TLS requirement is hard to exercise
// without standing up a server that refuses it, and easy to delete by accident.
func source(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("could not read %s: %v", name, err)
	}
	return string(b)
}

func TestARelayIsOffUntilItIsConfigured(t *testing.T) {
	t.Setenv("SMTP_RELAY_HOST", "")
	if Relaying() {
		t.Error("mail is being relayed with nothing configured")
	}
}

// A bare hostname gets the submission port. Somebody setting this is copying it
// out of a provider's docs, where it is usually written without one.
func TestABareHostGetsTheSubmissionPort(t *testing.T) {
	t.Setenv("SMTP_RELAY_HOST", "smtp.provider.example")
	if got := RelayHostname(); got != "smtp.provider.example:587" {
		t.Errorf("the relay is %q, want the submission port added", got)
	}
	t.Setenv("SMTP_RELAY_HOST", "smtp.provider.example:2525")
	if got := RelayHostname(); got != "smtp.provider.example:2525" {
		t.Errorf("a port that was given was changed to %q", got)
	}
	// Whitespace, because this comes out of an environment file.
	t.Setenv("SMTP_RELAY_HOST", "  smtp.provider.example  ")
	if got := RelayHostname(); got != "smtp.provider.example:587" {
		t.Errorf("the relay is %q", got)
	}
}

// The credential crosses this connection, so a server that will not encrypt is
// one to fail against rather than to send a password to.
func TestTheRelayRequiresSTARTTLS(t *testing.T) {
	src := source(t, "relay.go")
	if !strings.Contains(src, `c.Extension("STARTTLS")`) {
		t.Error("nothing checks that the relay offers STARTTLS")
	}
	if !strings.Contains(src, "does not offer STARTTLS") {
		t.Error("a relay with no STARTTLS is not refused")
	}
}
