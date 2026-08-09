package mail

// Mail to this instance is delivered here, not relayed to it.
//
// IsExternalEmail tested for an @ and nothing else, so asim@micro.mu on the
// instance that serves micro.mu counted as external. The send path relayed it
// out over SMTP, it arrived back at our own MX, and the anti-spoofing check
// refused it: "Sender address rejected: not authorized to send from this
// domain". Writing to yourself on your own server failed, and so did writing
// to one of your own agents at you+name@ — the address the product hands out
// and the one the inbound-agent trigger listens on.
//
// IsExternalAddress, three lines away in mail.go, had it right the whole time.

import (
	"os"
	"testing"
)

func withDomain(t *testing.T, d string) {
	t.Helper()
	prev := os.Getenv("MAIL_DOMAIN")
	os.Setenv("MAIL_DOMAIN", d)
	t.Cleanup(func() { os.Setenv("MAIL_DOMAIN", prev) })
}

func TestOurOwnDomainIsNotExternal(t *testing.T) {
	withDomain(t, "micro.mu")

	for _, local := range []string{
		"asim@micro.mu",
		"asim+claude@micro.mu", // an agent's address
		"ASIM@MICRO.MU",        // headers arrive in any case
		" asim@micro.mu ",
		"asim", // a bare username was always local
	} {
		if IsExternalEmail(local) {
			t.Errorf("%q was treated as external, so it is relayed out and bounces back", local)
		}
	}

	for _, external := range []string{"someone@example.com", "asim@aslam.me", "a@micro.mu.evil.com"} {
		if !IsExternalEmail(external) {
			t.Errorf("%q was treated as local, so it would never be sent", external)
		}
	}
}

// Whatever form the address is written in, it resolves to one account.
func TestALocalAddressResolvesToItsAccount(t *testing.T) {
	withDomain(t, "micro.mu")

	for _, c := range []struct{ in, want string }{
		{"asim", "asim"},
		{"asim@micro.mu", "asim"},
		{"asim+claude@micro.mu", "asim"},
		{"asim+claude", "asim"},
		{"  ASIM@micro.mu ", "asim"},
	} {
		if got := LocalRecipient(c.in); got != c.want {
			t.Errorf("LocalRecipient(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// An instance with no domain configured has no local domain to compare
// against, so anything with an @ is somewhere else.
func TestWithNoDomainEverythingAddressedIsExternal(t *testing.T) {
	withDomain(t, "")
	if !IsExternalEmail("someone@example.com") {
		t.Error("mail would never leave an instance with no MAIL_DOMAIN set")
	}
	if IsExternalEmail("asim") {
		t.Error("a bare username is always local")
	}
}
